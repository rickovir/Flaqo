# PRODUCT_SPEC.md

## Lightweight Feature Flag Platform

**Status:** MVP baseline\
**Product type:** Self-hostable / managed-ready feature flag management
platform\
**Primary goal:** Let an administrator manage Boolean feature flags per
project and environment, then propagate changes to connected
applications without redeployment.

------------------------------------------------------------------------

## 1. Product vision

Build a small, secure, developer-friendly feature flag platform. The
product intentionally focuses on the core feature-flag lifecycle rather
than experimentation or analytics.

The platform consists of: 1. A web dashboard for project, environment,
and flag management. 2. A Go backend that owns authentication,
configuration, and real-time delivery. 3. A TypeScript SDK that
applications use to fetch and evaluate flags locally.

**Core product promise:** change a flag in the dashboard; connected
applications receive the updated configuration without redeploying.

## 2. Target users and primary use cases

### Primary users

-   Developers integrating flags into frontend or backend applications.
-   Administrators managing project configuration.

### MVP use cases

-   Create a project.
-   Manage its Development, Staging, and Production environments.
-   Create a Boolean flag with a unique key.
-   Set the flag independently to ON or OFF in each environment.
-   Connect an application using an environment-scoped SDK key.
-   Receive flag changes through SSE.
-   Continue using the last valid configuration if the stream
    temporarily disconnects.

## 3. Explicit MVP scope

### In scope

**Project management** - Create, list, view, update, and delete
projects. - A project has a name and stable identifier. - Create the
default environments when a project is created: `development`,
`staging`, `production`. - Generate and revoke SDK credentials scoped to
a project and environment.

**Environment management** - List environments belonging to a project. -
Environment keys (`development`, `staging`, `production`) are fixed and
immutable. - MVP does not support arbitrary environment creation; the
initial three environments are sufficient. - Environment-specific flag
state is independent.

**Feature flag management** - Create a flag with a unique key within a
project. - Store an optional description. - Edit the description. -
Enable or disable the flag separately for each environment. - Delete a
flag. - Boolean values only.

**SDK and configuration delivery** - Fetch a full configuration snapshot
for one project/environment. - Evaluate flags locally in the SDK. -
Subscribe to updates through Server-Sent Events. - SSE sends a version
notification; SDK fetches the latest snapshot. - Keep the last known
good snapshot in memory during temporary network failure. - Support an
explicit default value when a flag is absent or the SDK has not
initialized. - A failed snapshot fetch is retried independently of
event arrival, so a client never stays stale while appearing connected.

**Basic security** - Custom dashboard authentication. - Secure password
hashing. - Server-managed sessions. - Read-only SDK credentials. -
Project/environment isolation. - Credential revocation. - Input
validation, rate limiting on authentication endpoints, HTTPS in
deployment, and secure cookie/CSRF controls.

### Out of scope

-   A/B testing, experimentation, statistical analysis, and analytics.
-   User targeting, segments, attributes, username/email lists, and
    identity tracking.
-   Percentage rollout or gradual rollout.
-   JSON, string, numeric, or multivariate flags.
-   Scheduling and automatic expiry.
-   Approval workflows and complex RBAC.
-   SSO, social login, organization invitations, and multi-tenant
    billing.
-   Advanced audit history and one-click rollback.
-   Redis, Kafka, WebSockets, microservices, and multi-region
    deployment.
-   Flag evaluation that depends on a network request for every
    `isEnabled` call.

## 4. Product concepts and terminology

-   **Project:** an application or product registered in the platform.
-   **Environment:** an isolated configuration scope within a project,
    initially Development, Staging, and Production.
-   **Feature flag:** a Boolean switch identified by a stable key.
-   **Flag state:** the ON/OFF value for a flag in a particular
    environment.
-   **SDK key:** a credential that grants read-only access to one
    project/environment configuration.
-   **Configuration version:** a monotonically increasing integer for
    one environment's configuration.
-   **SDK snapshot:** the complete set of flag values for one
    project/environment at a specific version.

A flag definition is shared across environments; its state is stored
separately for each environment.

## 5. Core user flows

### 5.1 First-time setup

1.  Administrator opens the dashboard.
2.  Administrator signs in.
3.  Administrator creates a project.
4.  The platform creates Development, Staging, and Production
    environments.
5.  Administrator generates an SDK key for the required environment.
6.  Developer installs and initializes the TypeScript SDK.

### 5.2 Create and toggle a flag

1.  Administrator opens a project.
2.  Administrator creates a flag, for example `new_dashboard`.
3.  The platform creates a default OFF state for each environment.
4.  Administrator toggles the flag ON in Development.
5.  Backend persists the change and increments that environment's
    configuration version.
6.  Connected SDK clients are notified through SSE and fetch the latest
    snapshot.
7.  The SDK atomically replaces its local snapshot and notifies
    application subscribers.

### 5.3 Disconnected SDK

1.  SDK has fetched a valid snapshot.
2.  SSE connection drops.
3.  SDK retains the last valid snapshot and continues local evaluation.
4.  SDK reconnects with its last observed version.
5.  SDK receives a version notification or detects that its version is
    behind.
6.  SDK fetches and applies the latest snapshot.

## 6. Functional requirements

  -----------------------------------------------------------------------
  ID                      Requirement             Priority
  ----------------------- ----------------------- -----------------------
  FR-001                  Administrator can sign  P0
                          in and sign out         

  FR-002                  Administrator session   P0
                          can expire and be       
                          revoked                 

  FR-003                  Administrator can       P0
                          create and manage       
                          projects                

  FR-004                  Project creation        P0
                          provisions the three    
                          default environments    

  FR-005                  Administrator can list  P0
                          project environments    

  FR-006                  Administrator can       P0
                          create a Boolean flag   

  FR-007                  Flag key is unique      P0
                          within a project        

  FR-008                  Administrator can edit  P0
                          flag description        

  FR-009                  Administrator can       P0
                          toggle a flag per       
                          environment             

  FR-010                  Administrator can       P0
                          delete a flag           

  FR-011                  SDK can fetch a         P0
                          complete environment    
                          configuration           

  FR-012                  SDK can subscribe to    P0
                          configuration changes   
                          over SSE                

  FR-013                  SDK evaluates flags     P0
                          locally without a       
                          network call            

  FR-014                  SDK preserves last      P0
                          known good              
                          configuration during    
                          disconnection           

  FR-015                  SDK credentials are     P0
                          scoped to one           
                          project/environment     

  FR-016                  Revoked SDK credentials P0
                          are rejected            

  FR-017                  Project and environment P0
                          boundaries are enforced 
                          server-side             

  FR-018                  Configuration versions  P0
                          increase on effective   
                          state changes           
  -----------------------------------------------------------------------

## 7. Non-functional requirements

-   **Correctness:** a successful toggle is durable before the platform
    notifies clients.
-   **Isolation:** credentials cannot read another project or
    environment.
-   **Availability behavior:** temporary SSE failure does not erase the
    SDK's last valid snapshot.
-   **Performance:** local flag evaluation is an in-memory lookup; it
    must not call the network.
-   **Maintainability:** dashboard, backend, and SDK are separate
    applications/packages in one repository.
-   **Portability:** use standard PostgreSQL features and keep
    Supabase-specific functionality out of business logic.
-   **Observability:** structured logs and basic health/readiness
    endpoints.
-   **Security:** never expose database credentials, password hashes,
    session tokens, or server SDK secrets to browser clients.

## 8. Dashboard information architecture

MVP screens: 1. Login. 2. Project list. 3. Create/edit project. 4.
Project detail with environment selector. 5. Feature flag list with key,
description, and ON/OFF toggle. 6. Create/edit flag. 7. SDK key
management for a project/environment.

Keep the dashboard deliberately simple. No analytics overview, charts,
experiment pages, or activity feed.

## 9. Acceptance criteria

The MVP is accepted when all of the following are demonstrated:

-   An administrator can sign in and create a project.
-   The project has Development, Staging, and Production environments.
-   An administrator can create a Boolean flag and toggle it
    independently in each environment.
-   An SDK key for one environment cannot read another environment or
    project. This is enforced by the database schema, not only by
    application code.
-   A connected SDK receives a change notification and fetches the new
    snapshot without page reload or redeployment.
-   SDK evaluation uses the local snapshot and performs no network
    request.
-   A disconnected SDK retains its last valid snapshot.
-   Reconnection eventually synchronizes to the latest configuration
    version.
-   A revoked SDK key can no longer fetch configuration or establish a
    stream, and its active streams are closed.
-   Concurrent reads and updates do not produce data races or partially
    applied snapshots.
-   The version reported with a configuration snapshot always matches
    the flag values in that same snapshot.
-   Database credentials and administrative credentials are never
    included in browser bundles or SDK configuration responses.

## 10. Product decisions intentionally deferred

The following decisions are not required to ship the MVP: - Multiple
administrators and fine-grained roles. - Organizations and tenant
billing. - Custom environment creation. - Flag archival instead of
deletion. - Audit log retention and rollback UI. - Targeting, segments,
and percentage rollout. - JSON or typed configuration values. -
Dedicated stream service and distributed event bus.
