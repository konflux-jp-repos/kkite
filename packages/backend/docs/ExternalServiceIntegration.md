# External Service Integration
If you don't plan on using a [custom controller](../../operator/docs/ControllerDevelopmentGuide.md) with an associated [custom webhook](./Webhooks.md), you can still make requests to Kite using external services.

Here are some examples:

#### CI/CD Pipeline Integration:
```yaml
# Tekton Task example
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: notify-kite
spec:
  params:
    - name: pipeline-name
    - name: status
    - name: failure-reason
  steps:
    - name: notify
      image: curlimages/curl
      script: |
        if [ "$(params.status)" = "Failed" ]; then
          curl -X POST http://kite-api/api/v1/webhooks/pipeline-failure \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer <access-token>" \
            -d '{
              "pipelineName": "$(params.pipeline-name)",
              "namespace": "$(context.taskRun.namespace)",
              "failureReason": "$(params.failure-reason)"
            }'
        else
          curl -X POST http://kite-api/api/v1/webhooks/pipeline-success \
            -H "Content-Type: application/json" \
            -d '{
              "pipelineName": "$(params.pipeline-name)",
              "namespace": "$(context.taskRun.namespace)"
            }'
        fi
```

#### Codebase integration

**Python**:
```python
import requests

def notify_kite_build_status(component_name, namespace, status, build_id, error_msg=None):
    base_url = "http://kite-api/api/v1/webhooks"

    if status == "failed":
        response = requests.post(f"{base_url}/build-failure", json={
            "componentName": component_name,
            "namespace": namespace,
            "buildId": build_id,
            "errorMessage": error_msg,
            "buildLogsUrl": f"https://builds.com/logs/{build_id}"
        })
    else:
        response = requests.post(f"{base_url}/build-success", json={
            "componentName": component_name,
            "namespace": namespace
        })

    return response.status_code == 200
```

**Go**:
```golang
// KiteClient handles integration with Kite API
type KiteClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewKiteClient creates a new Kite API client
func NewKiteClient(baseURL string) *KiteClient {
	return &KiteClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Build webhook request structures
type BuildFailureRequest struct {
	ComponentName string `json:"componentName"`
	Namespace     string `json:"namespace"`
	BuildID       string `json:"buildId"`
	ErrorMessage  string `json:"errorMessage"`
	BuildLogsURL  string `json:"buildLogsUrl,omitempty"`
}

type BuildSuccessRequest struct {
	ComponentName string `json:"componentName"`
	Namespace     string `json:"namespace"`
}

// Generic webhook response
type WebhookResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Issue   interface{} `json:"issue,omitempty"`
}

// ReportBuildFailure reports a build failure to Kite
func (k *KiteClient) ReportBuildFailure(ctx context.Context, req BuildFailureRequest) error {
	endpoint := fmt.Sprintf("%s/api/v1/webhooks/build-failure?namespace=%s", k.baseURL, req.Namespace)

	var response WebhookResponse
	if err := k.makeRequest(ctx, "POST", endpoint, req, &response); err != nil {
		return fmt.Errorf("failed to report build failure: %w", err)
	}

	log.Printf("Build failure reported successfully for component: %s", req.ComponentName)
	return nil
}

// ReportBuildSuccess reports a build success to Kite
func (k *KiteClient) ReportBuildSuccess(ctx context.Context, req BuildSuccessRequest) error {
	endpoint := fmt.Sprintf("%s/api/v1/webhooks/build-success?namespace=%s", k.baseURL, req.Namespace)

	var response WebhookResponse
	if err := k.makeRequest(ctx, "POST", endpoint, req, &response); err != nil {
		return fmt.Errorf("failed to report build success: %w", err)
	}

	log.Printf("Build success reported successfully for component: %s", req.ComponentName)
	return nil
}

// NotifyBuildStatus reports build status to Kite based on success/failure
func (k *KiteClient) NotifyBuildStatus(ctx context.Context, componentName, namespace, buildID string, success bool, errorMessage, logsURL string) error {
	if success {
		return k.ReportBuildSuccess(ctx, BuildSuccessRequest{
			ComponentName: componentName,
			Namespace:     namespace,
		})
	}

	return k.ReportBuildFailure(ctx, BuildFailureRequest{
		ComponentName: componentName,
		Namespace:     namespace,
		BuildID:       buildID,
		ErrorMessage:  errorMessage,
		BuildLogsURL:  logsURL,
	})
}
```
