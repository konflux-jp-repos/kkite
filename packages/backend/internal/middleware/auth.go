package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/konflux-ci/kite/internal/pkg/cache"
	"github.com/sirupsen/logrus"
	apiAuthnv1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const impersonateFlag = "AUTH_IMPERSONATE"

var ErrNoImpersonationData = errors.New("no impersonation data found")

type impersonatedData struct {
	resourceAttributes []*authv1.ResourceAttributes
	userInfo           user.Info
}

// Kubernetes namespaces access checker
type NamespaceChecker struct {
	client kubernetes.Interface
	logger *logrus.Logger
}

func NewNamespaceChecker(logger *logrus.Logger) (*NamespaceChecker, error) {
	// Try to create Kubernetes client

	// Attempt to get project local kubeconfig
	var kubeconfigPath string
	cwd, cwdErr := os.Getwd()
	if cwdErr == nil {
		kubeconfigPath = filepath.Join(cwd, "configs", "kube-config.yaml")
		logger.Infof("Using path %s", kubeconfigPath)
		if _, statErr := os.Stat(kubeconfigPath); statErr != nil {
			// Reset, look elsewhere
			kubeconfigPath = ""
		}
	}

	// Build config: prefer in-cluster -> local file -> default home
	config, err := rest.InClusterConfig()
	if err != nil {
		var cfgErr error
		if kubeconfigPath != "" {
			logger.Infof("Using project local kubeconfig: %s", kubeconfigPath)
			config, cfgErr = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		} else {
			logger.Info("No project local kubeconfig, falling back to ~/.kube/config")
			config, cfgErr = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		}
		if cfgErr != nil {
			logger.WithError(cfgErr).Warn("Failed to create a Kubernetes client, namespace check disabled")
		}
	}

	// Only create a clientset if we have a valid config
	if config == nil {
		logger.Warn("No valid kubernetes configuration found, namespace checking disabled")
		return &NamespaceChecker{client: nil, logger: logger}, nil
	}

	// Create clientset using config retrieved
	clientset, k8sCsErr := kubernetes.NewForConfig(config)
	if k8sCsErr != nil {
		logger.WithError(k8sCsErr).Warn("Failed to create Kubernetes clientset, namespace checking disabled")
		return &NamespaceChecker{client: nil, logger: logger}, nil
	}

	return &NamespaceChecker{client: clientset, logger: logger}, nil
}

func newDefaultInfoFromAuthN(info apiAuthnv1.UserInfo) user.Info {
	extra := make(map[string][]string)
	for k, v := range info.Extra {
		extra[k] = v // Explicit conversion
	}
	return &user.DefaultInfo{
		Name:   info.Username,
		UID:    info.UID,
		Groups: info.Groups,
		Extra:  extra,
	}
}

func extractBearerToken(header string) (string, error) {
	if header == "" {
		return "", errors.New("empty authorization bearer token given")
	}

	jwtToken := strings.Split(header, " ")
	if len(jwtToken) != 2 {
		return "", fmt.Errorf("incorrectly formatted authorization header, "+
			"expected two strings separated by a space but found %d", len(jwtToken))
	}

	return jwtToken[1], nil
}

func (nc *NamespaceChecker) Authentication(cache *cache.Cache, cacheExpirationAuthorized, cacheExpirationUnauthorized time.Duration) gin.HandlerFunc {
	tri := nc.client.AuthenticationV1().TokenReviews()
	return func(c *gin.Context) {
		token, err := extractBearerToken(c.GetHeader("Authorization"))
		if err != nil {
			c.Set("type", "publisher")
			c.Next()
			return
		}

		userInfo := cache.Get(token)
		if userInfo != nil {
			if userInfo == false { // Unauthenticated
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication failed"})
				c.Abort()
				return
			}

			c.Set("user", userInfo)
			c.Set("type", "consumer")
			c.Next()
			return
		}

		tr, err := tri.Create(c.Request.Context(), &apiAuthnv1.TokenReview{
			Spec: apiAuthnv1.TokenReviewSpec{
				Token: token,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "unexpected error on TokenReview"})
			c.Abort()
			return
		}
		if !tr.Status.Authenticated {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication failed"})
			c.Abort()
			cache.Set(token, false, cacheExpirationUnauthorized)
			return
		}

		userInfo = newDefaultInfoFromAuthN(tr.Status.User)
		cache.Set(token, userInfo, cacheExpirationAuthorized)

		c.Set("user", userInfo)
		c.Set("type", "consumer")
	}
}

func newImpersonatedData(c *gin.Context) (*impersonatedData, error) {

	userInfo := &user.DefaultInfo{}
	resourceAtts := make([]*authv1.ResourceAttributes, 0)
	var hasUser, hasGroups, hasUID, hasExtras bool

	userSarAtt, impersonatedUser, namespace := parseUser(c.Request.Header)
	if impersonatedUser != "" {
		hasUser = true
		userInfo.Name = impersonatedUser
		resourceAtts = append(resourceAtts, userSarAtt)
	}
	grpsSarAtts, groups := parseGroups(c.Request.Header)
	if len(groups) > 0 {
		hasGroups = true
		userInfo.Groups = groups
		resourceAtts = append(resourceAtts, grpsSarAtts...)
	} else if namespace != "" {
		// If no groups but it's a service account, the groups can be extracted from the namespace
		userInfo.Groups = serviceaccount.MakeGroupNames(namespace)
	}
	uidSarAtt, uid := parseUID(c.Request.Header)
	if uid != "" {
		hasUID = true
		userInfo.UID = uid
		resourceAtts = append(resourceAtts, uidSarAtt)
	}
	extrasSarAtts, extras := parseExtras(c.Request.Header)
	if len(extras) > 0 {
		hasExtras = true
		userInfo.Extra = extras
		resourceAtts = append(resourceAtts, extrasSarAtts...)
	}

	if !hasUser {
		if hasGroups || hasUID || hasExtras {
			return nil, fmt.Errorf("header %s required for impersonation", apiAuthnv1.ImpersonateUserHeader)
		}
		return nil, ErrNoImpersonationData
	}

	if userInfo.Name != user.Anonymous {
		// add 'system:authenticated' if it's not already provided and the user is not anonymous
		if !slices.Contains(groups, user.AllAuthenticated) {
			userInfo.Groups = append(userInfo.Groups, user.AllAuthenticated)
		}
	} else {
		// add 'system:unauthenticated' if it's not already provided and the user is anonymous
		if !slices.Contains(groups, user.AllUnauthenticated) {
			userInfo.Groups = append(userInfo.Groups, user.AllUnauthenticated)
		}
	}

	return &impersonatedData{resourceAttributes: resourceAtts, userInfo: userInfo}, nil
}

// parseUser returns the SAR ResourceAttribute for impersonation, the username and its namespace if it's a SA
// from HTTP impersonation headers
func parseUser(headers http.Header) (*authv1.ResourceAttributes, string, string) {
	impersonatedUser := headers.Get(apiAuthnv1.ImpersonateUserHeader)
	if impersonatedUser == "" {
		return nil, "", ""
	}
	namespace, _, err := serviceaccount.SplitUsername(impersonatedUser)

	// service account
	if err == nil {
		return &authv1.ResourceAttributes{
			Name:      impersonatedUser,
			Namespace: namespace,
			Resource:  "serviceaccounts",
			Verb:      "impersonate",
		}, impersonatedUser, namespace

	}

	// user
	return &authv1.ResourceAttributes{
		Name:     impersonatedUser,
		Resource: "users",
		Verb:     "impersonate",
	}, impersonatedUser, ""

}

// parseGroups returns the SAR ResourceAttributes for impersonation and the groups from HTTP impersonation headers
func parseGroups(headers http.Header) ([]*authv1.ResourceAttributes, []string) {
	groups := headers.Values(apiAuthnv1.ImpersonateGroupHeader)
	resourceAtts := make([]*authv1.ResourceAttributes, 0)
	for _, group := range groups {
		resourceAtts = append(resourceAtts, &authv1.ResourceAttributes{
			Name:     group,
			Resource: "groups",
			Verb:     "impersonate",
		})
	}
	return resourceAtts, groups
}

// parseUID returns the SAR ResourceAttribute for impersonation and the UID from HTTP impersonation headers
func parseUID(headers http.Header) (*authv1.ResourceAttributes, string) {
	uid := headers.Get(apiAuthnv1.ImpersonateUIDHeader)
	if uid == "" {
		return nil, ""
	}
	return &authv1.ResourceAttributes{
		Group:    apiAuthnv1.SchemeGroupVersion.Group,
		Name:     uid,
		Resource: "uids",
		Verb:     "impersonate",
	}, uid
}

// parseExtras returns the SAR ResourceAttributes for impersonation and the extras from HTTP impersonation headers
func parseExtras(headers http.Header) ([]*authv1.ResourceAttributes, map[string][]string) {
	extras := make(map[string][]string)
	resourceAtts := make([]*authv1.ResourceAttributes, 0)
	for headerKey, headerValues := range headers {
		if strings.HasPrefix(headerKey, apiAuthnv1.ImpersonateUserExtraHeaderPrefix) {
			encodedKey := strings.TrimPrefix(headerKey, apiAuthnv1.ImpersonateUserExtraHeaderPrefix)
			key, err := url.PathUnescape(encodedKey)
			if err != nil {
				key = encodedKey
			}
			for _, value := range headerValues {
				extras[key] = append(extras[key], value)
				resourceAtts = append(resourceAtts, &authv1.ResourceAttributes{
					Group:       apiAuthnv1.SchemeGroupVersion.Group,
					Name:        value,
					Resource:    "userextras",
					Subresource: key,
					Verb:        "impersonate",
				})
			}
		}
	}
	return resourceAtts, extras
}

func (nc *NamespaceChecker) Impersonation(
	cache *cache.Cache,
	cacheExpirationAuthorized,
	cacheExpirationUnauthorized time.Duration) gin.HandlerFunc {

	if os.Getenv(impersonateFlag) != "true" {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return func(c *gin.Context) {
		user_type, _ := c.Get("type")
		if user_type == "publisher" {
			c.Next()
			return
	}
		imp, imperErr := newImpersonatedData(c)
		if imperErr != nil && !errors.Is(imperErr, ErrNoImpersonationData) {
			c.JSON(http.StatusBadRequest, gin.H{"error": imperErr})
			c.Abort()
			return
		}
		// No impersonated data so this middleware is skipped
		if imp == nil {
			c.Next()
			return
		}

		requester, ok := c.Get("user")
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found in context"})
			c.Abort()
			return
		}
		requesterInfo, okCast := requester.(*user.DefaultInfo)
		if !okCast {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unexpected user type in context"})
			c.Abort()
			return
		}
		for _, resourceAttribute := range imp.resourceAttributes {
			accessReview := &authv1.SubjectAccessReview{
				Spec: authv1.SubjectAccessReviewSpec{
					User: requesterInfo.GetName(),
					UID: requesterInfo.GetUID(),
					Groups: requesterInfo.GetGroups(),
					ResourceAttributes: resourceAttribute,
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), cacheExpirationAuthorized)
			defer cancel()

			_, err := nc.client.AuthorizationV1().SubjectAccessReviews().Create(
				ctx, accessReview, metav1.CreateOptions{})

			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "User don't have permission to impersonate"})
				c.Abort()
				return
			}
		}
		// The context user is updated with the impersonated user info
		c.Set("user", imp.userInfo)
	}
}

func (nc *NamespaceChecker) CheckNamespacessAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get namespaces from params, body or query
		namespace := c.Param("namespace")
		if namespace == "" {
			namespace = c.Query("namespace")
		}
		if namespace == "" {
			// Try to get from request body
			if c.Request.Method == "POST" || c.Request.Method == "PUT" {
				if body, exists := c.Get("requestBody"); exists {
					if bodyMap, ok := body.(map[string]interface{}); ok {
						if ns, ok := bodyMap["namespace"].(string); ok {
							namespace = ns
						}
					}
				}
			}
		}

		if namespace == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing namespace"})
			c.Abort()
			return
		}

		// If K8s client is not available, skip check
		if nc.client == nil {
			nc.logger.Debug("Kubernetes client not available, skipping namespace access check")
			c.Next()
			return
		}

		requester, ok := c.Get("user")
		if ok {
			requesterInfo, okCast := requester.(*user.DefaultInfo)
			if !okCast {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Unexpected user type in context"})
				c.Abort()
				return
			}
			// Check if user has access to the namespace by checking if they can get pods
			if err := nc.checkUserPodAccess(namespace, requesterInfo); err != nil {
				nc.logger.WithError(err).WithField("namespace", namespace).Warn("Access Denied")
				c.JSON(http.StatusForbidden, gin.H{"error": "Access denied to this namespace"})
				c.Abort()
				return
			}
		} else {
			// Check if Kite SA has access to the namespace by checking if they can get pods
			if err := nc.checkPodAccess(namespace); err != nil {
				nc.logger.WithError(err).WithField("namespace", namespace).Warn("Access Denied")
				c.JSON(http.StatusForbidden, gin.H{"error": "Access denied to this namespace"})
				c.Abort()
				return
			}
		}

		nc.logger.WithField("namespace", namespace).Debug("Access allowed")
		c.Next()
	}
}

func (nc *NamespaceChecker) checkPodAccess(namespace string) error {
	if nc.client == nil {
		return nil // Skip check if client is not available
	}

	// Create a SelfSubjectAccessReview to check if the user can get pods in the namespace
	accessReview := &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      "get",
				Resource:  "pods",
			},
		},
	}

	// Run the access review for max 10 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := nc.client.AuthorizationV1().SelfSubjectAccessReviews().Create(
		ctx, accessReview, metav1.CreateOptions{})

	if err != nil {
		return fmt.Errorf("failed to check kite namespace access: %w", err)
	}

	if !result.Status.Allowed {
		return fmt.Errorf("access denied for kite to namespace %s", namespace)
	}

	return nil
}

func (nc *NamespaceChecker) checkUserPodAccess(namespace string, requester user.Info) error {
	if nc.client == nil {
		return nil // Skip check if client is not available
	}

	// Create a SubjectAccessReview to check if the user can get pods in the namespace
	accessReview := &authv1.SubjectAccessReview{
		Spec: authv1.SubjectAccessReviewSpec{
			User: requester.GetName(),
			UID: requester.GetUID(),
			Groups: requester.GetGroups(),
			ResourceAttributes: &authv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      "get",
				Resource:  "pods",
			},
		},
	}

	// Run the access review for max 10 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := nc.client.AuthorizationV1().SubjectAccessReviews().Create(
		ctx, accessReview, metav1.CreateOptions{})

	if err != nil {
		return fmt.Errorf("failed to check user namespace access: %w", err)
	}

	if !result.Status.Allowed {
		return fmt.Errorf("access denied for %s to namespace %s", requester.GetName(), namespace)
	}

	return nil
}
