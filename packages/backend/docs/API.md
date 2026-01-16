# Konflux Issues API Documentation

## Table of Contents
- [Overview](#overview)
- [Authentication & Authorization](#authentication--authorization)
- [Data Models](#data-models)
- [API Endpoints](#api-endpoints)

---

## Overview

The Konflux Issues API is a RESTful service for managing and tracking issues within Konflux CI/CD pipelines, components and external tooling.

It provides endpoints for creating, reading, updating, and deleting issues, as well as custom webhook endpoints for automated issue management.

The goal of this project is for this service to be the backend to the Konflux Issues Dashboard.

The Konflux Issues Dashboard will function like a car dashboard - a centralized place to view and monitor issues (Specifically issues related to building and shipping applications in Konflux).

---

## Authentication & Authorization

The API will use Kubernetes RBAC for namespace-based access control (**Work In Progress**). Users must have access to the Kubernetes namespace to interact with issues in that namespace.

The user or service account consuming Kite API must send the `Authorization` header as bearer token where value should be its access token.

```bash
curl --request GET \
    --url 'https://kite.service' \
    --header 'Authorization: Bearer <access token>'
    <COMMAND_TAIL>
```

---

## Data Models

### Issue

An **issue** represents a problem or concern in the system. This problem can be related to resources in the Konflux (failed PipelineRuns, builds, etc) or outside of the Konflux cluster (MintMaker).

```json
{
  "id": "uuid",
  "title": "string",
  "description": "string",
  "severity": "info|minor|major|critical",
  "issueType": "build|test|release|dependency|pipeline",
  "state": "ACTIVE|RESOLVED",
  "detectedAt": "2025-01-01T12:00:00Z",
  "resolvedAt": "2025-01-01T13:00:00Z",
  "namespace": "string",
  "scopeId": "uuid",
  "scope": {
    "id": "uuid",
    "resourceType": "string",
    "resourceName": "string",
    "resourceNamespace": "string"
  },
  "links": [
    {
      "id": "uuid",
      "title": "string",
      "url": "string",
      "issueId": "uuid"
    }
  ],
  "relatedFrom": [],
  "relatedTo": [],
  "createdAt": "2025-01-01T12:00:00Z",
  "updatedAt": "2025-01-01T12:00:00Z"
}
```

### Enums

**Severity:**
- `info` - Informational issues
- `minor` - Minor issues that don't block functionality
- `major` - Major issues that impact functionality
- `critical` - Critical issues that block functionality

**Issue Type:**
- `build` - Build-related issues
- `test` - Test-related issues
- `release` - Release and deployment issues
- `dependency` - Dependency-related issues
- `pipeline` - Pipeline execution issues

**State:**
- `ACTIVE` - Issue is currently active/unresolved
- `RESOLVED` - Issue has been resolved

---

## API Endpoints

### Health & System

#### GET /api/v1/health/
Returns service health status.

**Response:**
```json
{
  "status": "UP",
  "message": "All systems operational",
  "timestamp": "2025-07-31T17:12:07.741010936Z",
  "components": {
    "api": {
      "status": "UP",
      "message": "API server is responding",
      "details": {
        "version": "0.0.1"
      }
    },
    "database": {
      "status": "UP",
      "message": "Database connection successful",
      "details": {
        "connection_status": "Healthy",
        "response_time_seconds": 0.000405068,
        "open_connections": 1,
        "idle_connections": 1,
        "max_open_connections": 100
      }
    },
    "response_time": {
      "status": "UP",
      "message": "Response time measurement",
      "details": {
        "duration_seconds": 0.000420555
      }
    }
  }
}
```

#### GET /api/v1/version
Returns service version information.

**Response:**
```json
{
  "version": "1.0.0",
  "name": "Konflux Issues API",
  "description": "API for managing issues in Konflux"
}
```

---

### Issues

#### GET /api/v1/issues
Retrieve a list of issues with optional filtering.

**Query Parameters:**
- `namespace` (required) - Kubernetes namespace
- `severity` (optional) - Filter by severity: `info|minor|major|critical`
- `issueType` (optional) - Filter by type: `build|test|release|dependency|pipeline`
- `state` (optional) - Filter by state: `ACTIVE|RESOLVED`
- `resourceType` (optional) - Filter by resource type
- `resourceName` (optional) - Filter by resource name
- `search` (optional) - Search in title and description
- `limit` (optional, default: 50) - Number of results to return
- `offset` (optional, default: 0) - Number of results to skip

**Example Request:**
```bash
GET /api/v1/issues?namespace=team-alpha&severity=critical&limit=10
```

**Response:**
```json
{
  "data": [
    {
      "id": "123e4567-e89b-12d3-a456-426614174000",
      "title": "Frontend build failed due to dependency conflict",
      "description": "The build process failed...",
      "severity": "major",
      "issueType": "build",
      "state": "ACTIVE",
      "detectedAt": "2025-01-01T12:00:00Z",
      "namespace": "team-alpha",
      "scope": {
        "resourceType": "component",
        "resourceName": "frontend-ui",
        "resourceNamespace": "team-alpha"
      },
      "links": [],
      "createdAt": "2025-01-01T12:00:00Z",
      "updatedAt": "2025-01-01T12:00:00Z"
    }
  ],
  "total": 1,
  "limit": 10,
  "offset": 0
}
```

#### POST /api/v1/issues
Create a new issue.

**Request Body:**
```json
{
  "title": "string (required)",
  "description": "string (required)",
  "severity": "info|minor|major|critical (required)",
  "issueType": "build|test|release|dependency|pipeline (required)",
  "state": "ACTIVE|RESOLVED (optional, default: ACTIVE)",
  "namespace": "string (required)",
  "scope": {
    "resourceType": "string (required)",
    "resourceName": "string (required)",
    "resourceNamespace": "string (optional, defaults to namespace)"
  },
  "links": [
    {
      "title": "string (required)",
      "url": "string (required)"
    }
  ]
}
```

**Response:** `201 Created`
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "title": "Frontend build failed",
  // ... full issue object
}
```

#### GET /api/v1/issues/:id
Retrieve a specific issue by ID.

**Path Parameters:**
- `id` (required) - Issue UUID

**Query Parameters:**
- `namespace` (optional) - Namespace for access control

**Response:** `200 OK`
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "title": "Frontend build failed",
  // ... full issue object with related issues and links
}
```

**Error Responses:**
- `404 Not Found` - Issue not found
- `403 Forbidden` - Access denied to namespace

#### PUT /api/v1/issues/:id
Update an existing issue.

**Path Parameters:**
- `id` (required) - Issue UUID

**Query Parameters:**
- `namespace` (optional) - Namespace for access control

**Request Body (all fields optional):**
```json
{
  "title": "string",
  "description": "string",
  "severity": "info|minor|major|critical",
  "issueType": "build|test|release|dependency|pipeline",
  "state": "ACTIVE|RESOLVED",
  "resolvedAt": "2025-01-01T13:00:00Z",
  "links": [
    {
      "title": "string (required)",
      "url": "string (required)"
    }
  ]
}
```

**Response:** `200 OK`
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  // ... updated issue object
}
```

#### DELETE /api/v1/issues/:id
Delete an issue and all related data.

**Path Parameters:**
- `id` (required) - Issue UUID

**Query Parameters:**
- `namespace` (optional) - Namespace for access control

**Response:** `204 No Content`

#### POST /api/v1/issues/:id/resolve
Mark an issue as resolved.

**Path Parameters:**
- `id` (required) - Issue UUID

**Query Parameters:**
- `namespace` (optional) - Namespace for access control

**Response:** `200 OK`
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "state": "RESOLVED",
  "resolvedAt": "2025-01-01T13:00:00Z",
  // ... full updated issue object
}
```

#### POST /api/v1/issues/:id/related
Create a relationship between two issues.

**Path Parameters:**
- `id` (required) - Source issue UUID

**Request Body:**
```json
{
  "relatedId": "uuid (required)"
}
```

**Response:** `201 Created`
```json
{
  "message": "Relationship created"
}
```

**Error Responses:**
- `404 Not Found` - One or both issues not found
- `409 Conflict` - Relationship already exists

#### DELETE /api/v1/issues/:id/related/:relatedId
Remove a relationship between issues.

**Path Parameters:**
- `id` (required) - Source issue UUID
- `relatedId` (required) - Target issue UUID

**Response:** `204 No Content`
