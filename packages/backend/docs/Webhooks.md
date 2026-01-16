# Webhooks

## Table of Contents

- [Overview](#overview)
- [Custom Webhook Endpoints](#custom-webhook-endpoints)
  - [Example Webhook Endpoints](#example-webhook-endpoints)
    - [Pipeline Failure Webhook](#pipeline-failure-webhook)
    - [Pipeline Success Webhook](#pipeline-success-webhook)
- [Creating Custom Webhook Endpoints](#creating-custom-webhook-endpoints)
  - [Example: Build Failure](#example-build-failure)
  - [Example: Deployment Failure](#example-deployment-failure)
  - [Example: Security Scan Failure](#example-security-scan-failure)
- [Issue Grouping with Scope Objects](#issue-grouping-with-scope-objects)
  - [Scope Object Structure](#scope-object-structure)
  - [Common Resource Types](#common-resource-types)
  - [How Grouping Works](#how-grouping-works)
  - [Benefits of Scope-Based Grouping](#benefits-of-scope-based-grouping)
  - [Automatic Issue Resolution (via custom webhooks)](#automatic-issue-resolution-via-custom-webhooks)
- [Developing Custom Webhooks](#developing-custom-webhooks)

## Overview
Kite allows any service that can send HTTP requests to create and manage issues

There are two main ways to send requests:
1. **Standard Issues API** - Use the standard REST API endpoints
2. **Custom Webhook Endpoints** - Develop and use specialized webhook endpoints that are customized for your use case.

---

## Custom Webhook Endpoints

Webhook endpoints provide a simple way to create and resolve issues. They handle the complexity of issue creation, duplicate checking and automatic resolution.

_Obs: Only authenticated user can post issues, so in order to post any issue the user, or service account, must send the its access token as bearer type of authentication in request_

### Example Webhook endpoints
The following example shows webhook endpoints for Tekton Pipeline failures and successes.
#### Pipeline Failure Webhook
**Endpoint**: `POST /api/v1/webhooks/pipeline-failure`

Creates issues when pipelines fail. Automatically handles duplicate checking and updates existing issues related to that pipeline failure.

**Request Payload**:
```json
{
  "pipelineName": "frontend-build",
  "namespace": "team-alpha",
  "failureReason": "Dependency conflict with React version",
  "runId": "run-123",
  "logsUrl": "https://your-ci.com/logs/run-123"
}
```

**What it does**:
- Creates an issue with title "Pipeline run failed: frontend-build"
- Sets issue type to "pipeline" and severity "major"
- Links to pipeline logs for easy debugging
- If a similar issue already exists for the same pipeline, it updates that issue instead of creating a duplicate

Internally the issue generated from that payload looks something like this:
```json
{
	"id": "986d686c-bce6-44be-b6ba-a7b5b88eec58",
	"title": "Pipeline run failed: frontend-build",
	"description": "The pipeline run frontend-build failed with reason: Dependency conflict with React version",
	"severity": "major",
	"issueType": "pipeline",
	"state": "ACTIVE",
	"detectedAt": "2025-06-17T18:13:29.007244Z",
	"resolvedAt": null,
	"namespace": "team-alpha",
	"scopeId": "1a483caf-f349-4a9d-879a-df74a2b55eb3",
	"scope": {
		"id": "1a483caf-f349-4a9d-879a-df74a2b55eb3",
		"resourceType": "pipelinerun",
		"resourceName": "frontend-build",
		"resourceNamespace": "team-alpha"
	},
	"links": [
		{
			"id": "d4ff5876-8c88-4703-9836-f6ac66e73ba0",
			"title": "Pipeline Run Logs",
			"url": "https://your-ci.com/logs/run-123",
			"issueId": "986d686c-bce6-44be-b6ba-a7b5b88eec58"
		}
	],
	"relatedFrom": [],
	"relatedTo": [],
	"createdAt": "2025-06-17T18:13:29.008239Z",
	"updatedAt": "2025-06-17T18:13:29.008239Z"
}
```

---

### Pipeline Success Webhook
**Endpoint**: `POST /api/v1/webhooks/pipeline-success`
Automatically resolves pipeline issues when pipelines succeed.

**Request Payload**:
```json
{
  "pipelineName": "frontend-build",
  "namespace": "team-alpha"
}
```

**What it does**:
- Finds all active issues related to the specified `pipelineName` in `namespace` "team-alpha"
- Marks them as "RESOLVED"
- Sets the resolution timestamp

After hitting this endpoint, the issue created from the failure endpoint will be updated:
```json
{
	"id": "986d686c-bce6-44be-b6ba-a7b5b88eec58",
	"title": "Pipeline run failed: frontend-build",
	"description": "The pipeline run frontend-build failed with reason: Dependency conflict with React version",
	"severity": "major",
	"issueType": "pipeline",
	"state": "RESOLVED",
	"detectedAt": "2025-06-17T18:13:29.007244Z",
	"resolvedAt": "2025-06-17T18:32:36.107527Z",
	"namespace": "team-alpha",
	"scopeId": "1a483caf-f349-4a9d-879a-df74a2b55eb3",
	"scope": {
		"id": "1a483caf-f349-4a9d-879a-df74a2b55eb3",
		"resourceType": "pipelinerun",
		"resourceName": "frontend-build",
		"resourceNamespace": "team-alpha"
	},
	"links": [
		{
			"id": "d4ff5876-8c88-4703-9836-f6ac66e73ba0",
			"title": "Pipeline Run Logs",
			"url": "https://your-ci.com/logs/run-123",
			"issueId": "986d686c-bce6-44be-b6ba-a7b5b88eec58"
		}
	],
	"relatedFrom": [],
	"relatedTo": [],
	"createdAt": "2025-06-17T18:13:29.008239Z",
	"updatedAt": "2025-06-17T18:32:36.107527Z"
}
```

---

## Creating Custom Webhook Endpoints
You can create custom webhook endpoints for your specific workflow that augment the standard Issues payload shown in the [API](./API.md) docs.

Here are some example endpoints and payloads:

### Example: Build Failure
```json
// POST /api/v1/webhooks/build-failure
{
  "componentName": "user-service",
  "namespace": "team-beta",
  "buildId": "build-456",
  "errorMessage": "Compilation failed: missing dependency",
  "buildLogsUrl": "https://build-system.com/logs/456"
}
```

---

### Example: Deployment Failure
```json
// POST /api/v1/webhooks/deployment-failure
{
	"applicationName": "e-commerce-app",
  "namespace": "team-gamma",
  "deploymentId": "deploy-789",
  "failureReason": "Resource quota exceeded",
  "dashboardUrl": "https://dashboard.com/deployments/789"
}
```

---

### Example: Security Scan Failure
```json
// POST /api/v1/webhooks/security-scan-failure
{
  "componentName": "api-gateway",
  "namespace": "team-delta",
  "scanId": "scan-321",
  "vulnerabilities": ["CVE-2024-1234", "CVE-2024-5678"],
  "reportUrl": "https://security.com/reports/321"
}
```

---

## Issue Grouping with Scope Objects
The `scope` object is the key to how Kite groups and manages related issues. It defines what resource an issue is related to.

### Scope Object Structure
```json
{
	"resourceType": "component", // What kind of resource
	"resourceName": "user-service", // Specific resource name
	"resourceNamespace": "team-beta", // Where the resource lives
}
```

### Common Resource Types
- `pipelinerun` - Issues related to pipeline executions
- `component` - Issues with application components
- `application` - Issues with entire applications
- `workspace` - Issues affecting workspaces
- `environment` - Environment-specific issues

---

### How grouping works

Issues with the same scope are considered related:
```json
// These two issues will be grouped together because they have the same scope
{
  "title": "Build failed for user-service",
	...
  "scope": {
    "resourceType": "component",
    "resourceName": "user-service",
    "resourceNamespace": "team-beta"
  }
	...
}

{
  "title": "Tests failing for user-service",
	...
  "scope": {
    "resourceType": "component",
    "resourceName": "user-service",
    "resourceNamespace": "team-beta"
  }
	...
}
```

---

### Benefits of Scope-Based Grouping

- **Prevents Duplicates** - Won't create multiple active issues for the same resource
- **Easy Filtering** - Find all issues for a specific component or application
- **Automatic Resolution** - Resolve all issues for a resource when it's fixed
- **Better Organization** - Issues are naturally organized by what they affect

---

### Automatic Issue Resolution (via custom webhooks):

Automatic resolution uses the `scope` object to find and resolve related issues when problems are fixed.

Going back to the pipeline webhook example, lets create two issues with the same `scope`:

**Payloads**
```json
// POST /api/v1/webhooks/pipeline-failure

// Payload 1
{
  "pipelineName": "frontend-build",
  "namespace": "team-alpha",
  "failureReason": "Dependency conflict with React version",
  "runId": "run-123",
  "logsUrl": "https://your-ci.com/logs/run-123"
}

// Payload 2
{
  "pipelineName": "frontend-build",
  "namespace": "team-alpha",
  "failureReason": "Yarn version outdated, needs update",
  "runId": "run-456",
  "logsUrl": "https://your-ci.com/logs/run-456"
}
```

These requests would generate the following issues:
```json
// From Payload 1
{
	"id": "4a99b4a0-fbde-4afd-9bfc-5307178ababb",
	"title": "Pipeline run failed: frontend-build",
	"description": "The pipeline run frontend-build failed with reason: Dependency conflict with React version",
	"severity": "major",
	"issueType": "pipelinerun",
	"state": "ACTIVE",
	"detectedAt": "2025-06-17T19:49:31.248829Z",
	"resolvedAt": null,
	"namespace": "team-alpha",
	"scopeId": "abb27ca3-fd9d-48e1-8da7-0a776a4915d1",
	"scope": {
		"id": "abb27ca3-fd9d-48e1-8da7-0a776a4915d1",
		"resourceType": "pipelinerun",
		"resourceName": "frontend-build",
		"resourceNamespace": "team-alpha"
	},
	"links": [
		{
			"id": "8eb2802b-a3c4-4158-8df2-73396c646572",
			"title": "Pipeline Run Logs",
			"url": "https://your-ci.com/logs/run-123",
			"issueId": "4a99b4a0-fbde-4afd-9bfc-5307178ababb"
		}
	],
	"relatedFrom": [],
	"relatedTo": [],
	"createdAt": "2025-06-17T19:49:31.249607Z",
	"updatedAt": "2025-06-17T19:49:31.249607Z"
},
// From Payload 2
{
	"id": "58c96080-aba8-40c3-a9e9-585a5fd64692",
	"title": "Pipeline run failed: frontend-build",
	"description": "The pipeline run frontend-build failed with reason: Yarn version outdated, needs update",
	"severity": "major",
	"issueType": "pipelinerun",
	"state": "ACTIVE",
	"detectedAt": "2025-06-17T19:36:07.860624Z",
	"resolvedAt": null,
	"namespace": "team-alpha",
	"scopeId": "390e6c1d-7078-4581-9d35-8c49bce42301",
	"scope": {
		"id": "390e6c1d-7078-4581-9d35-8c49bce42301",
		"resourceType": "pipelinerun",
		"resourceName": "frontend-build",
		"resourceNamespace": "team-alpha"
	},
	"links": [
		{
			"id": "abb548f7-16a8-465e-94d4-6ec06fcfdef9",
			"title": "Pipeline Run Logs",
			"url": "https://your-ci.com/logs/run-456",
			"issueId": "58c96080-aba8-40c3-a9e9-585a5fd64692"
		}
	],
	"relatedFrom": [],
	"relatedTo": [],
	"createdAt": "2025-06-17T19:36:07.861097Z",
	"updatedAt": "2025-06-17T19:42:06.214236Z"
}
```

Note that both records have the same scope:
```json
...
	"scope": {
		"id": "390e6c1d-7078-4581-9d35-8c49bce42301",
		"resourceType": "pipelinerun",
		"resourceName": "frontend-build",
		"resourceNamespace": "team-alpha"
	},
...
```

**NOTE:** Because this webhook endpoint is customized for pipeline runs, the `resourceType` is automatically set to `pipelinerun`. Only the **pipeline name** and **namespace** is needed in the payload.

When the pipeline with the name `frontend-build` passes, both these issues should get resolved:

**Request Payload**
```json
// POST /api/v1/webhooks/pipeline-success
{
  "pipelineName": "frontend-build",
  "namespace": "team-alpha"
}
```

**Response**
```json
{
	"message": "Resolved 2 issue(s) for pipeline frontend-build",
	"status": "success"
}
```

**Updated issues**:
```json
// Payload 1
{
	"id": "4a99b4a0-fbde-4afd-9bfc-5307178ababb",
	"title": "Pipeline run failed: frontend-build",
	"description": "The pipeline run frontend-build failed with reason: Dependency conflict with React version",
	"severity": "major",
	"issueType": "pipelinerun",
	"state": "RESOLVED",
	"detectedAt": "2025-06-17T19:49:31.248829Z",
	"resolvedAt": "2025-06-17T19:57:19.634641Z",
	"namespace": "team-alpha",
	"scopeId": "abb27ca3-fd9d-48e1-8da7-0a776a4915d1",
	"scope": {
		"id": "abb27ca3-fd9d-48e1-8da7-0a776a4915d1",
		"resourceType": "pipelinerun",
		"resourceName": "frontend-build",
		"resourceNamespace": "team-alpha"
	},
	"links": [
		{
			"id": "8eb2802b-a3c4-4158-8df2-73396c646572",
			"title": "Pipeline Run Logs",
			"url": "https://your-ci.com/logs/run-123",
			"issueId": "4a99b4a0-fbde-4afd-9bfc-5307178ababb"
		}
	],
	"relatedFrom": [],
	"relatedTo": [],
	"createdAt": "2025-06-17T19:49:31.249607Z",
	"updatedAt": "2025-06-17T19:57:19.634641Z"
},
// Payload 2
{
	"id": "58c96080-aba8-40c3-a9e9-585a5fd64692",
	"title": "Pipeline run failed: frontend-build",
	"description": "The pipeline run frontend-build failed with reason: Yarn version outdated, needs update",
	"severity": "major",
	"issueType": "pipelinerun",
	"state": "RESOLVED",
	"detectedAt": "2025-06-17T19:36:07.860624Z",
	"resolvedAt": "2025-06-17T19:57:19.634641Z",
	"namespace": "team-alpha",
	"scopeId": "390e6c1d-7078-4581-9d35-8c49bce42301",
	"scope": {
		"id": "390e6c1d-7078-4581-9d35-8c49bce42301",
		"resourceType": "pipelinerun",
		"resourceName": "frontend-build",
		"resourceNamespace": "team-alpha"
	},
	"links": [
		{
			"id": "abb548f7-16a8-465e-94d4-6ec06fcfdef9",
			"title": "Pipeline Run Logs",
			"url": "https://your-ci.com/logs/run-456",
			"issueId": "58c96080-aba8-40c3-a9e9-585a5fd64692"
		}
	],
	"relatedFrom": [],
	"relatedTo": [],
	"createdAt": "2025-06-17T19:36:07.861097Z",
	"updatedAt": "2025-06-17T19:57:19.634641Z"
}
```

---

### Developing Custom Webhooks
To develop a custom webhook endpoint for your use case:
1. **Identify Your Workflow** - What events do you want to track?
2. **Define the Scope** - What resources are affected by these events?
3. **Specify the Data** - What information needs to be captured?
4. **Plan Resolution** - How will issues be automatically resolved? (this should be handled in the [custom controller](../../operator/docs/ControllerDevelopmentGuide.md) for your resource)

Here is a template to help with the request:
```markdown
## Webhook Request: [Workflow Name]

**Purpose:** Track [type of issues] for [system/service]

**Failure Endpoint:** POST /api/v1/webhooks/[workflow-name]-failure
**Success Endpoint:** POST /api/v1/webhooks/[workflow-name]-success

**Failure Request Body:**
{
  "resourceName": "string",
  "namespace": "string",
  "failureReason": "string",
  // ... other relevant fields
}

**Success Request Body**:
{
  "resourceName": "string",
  "namespace": "string"
}

**Scope Mapping**:
- resourceType: "[your-resource-type]"
- resourceName: from request body
- resourceNamespace: from request body

**Issue Details**:
- issueType: "[build|test|release|dependency|pipeline]"
- severity: "[info|minor|major|critical]"
- title format: "[Your title template]"

```

---
