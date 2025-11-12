# API Gateway Documentation

## Table of Contents
1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Features Implemented](#features-implemented)
4. [How It Works](#how-it-works)
5. [API Endpoints](#api-endpoints)
6. [Testing Guide](#testing-guide)
7. [Adding New Services](#adding-new-services)
8. [Configuration](#configuration)
9. [Troubleshooting](#troubleshooting)

---

## Overview

The API Gateway is a NestJS-based microservices gateway that serves as the **single entry point** for all client requests in the notification system. It handles:

- **Authentication & Authorization**: JWT token validation and user context management
- **Request Routing**: Proxies requests to appropriate microservices (user-service, template-service)
- **Request Validation**: Validates incoming requests using DTOs
- **Rate Limiting**: Protects services from abuse using Redis-based throttling
- **Notification Processing**: Handles notification requests, template rendering, and queue routing
- **Status Tracking**: Tracks notification lifecycle in Redis
- **Error Handling**: Centralized error handling with consistent response format
- **Logging**: Comprehensive request/response logging

---

## Architecture

### System Components

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │
       ▼
┌─────────────────────────────────────┐
│         API Gateway (Port 3000)      │
│  ┌──────────────────────────────┐ │
│  │  JWT Authentication            │ │
│  │  Request Validation            │ │
│  │  Rate Limiting                 │ │
│  │  Proxy Middleware              │ │
│  │  Notification Service          │ │
│  └──────────────────────────────┘ │
└──────┬──────────────────┬──────────┘
       │                  │
       ▼                  ▼
┌─────────────┐    ┌──────────────┐
│ User Service│    │Template      │
│ (Port 3001) │    │Service       │
│             │    │(Port 3003)    │
└─────────────┘    └──────────────┘
       │                  │
       └────────┬─────────┘
                ▼
         ┌─────────────┐
         │   Redis     │
         │  (Queues &  │
         │   Status)   │
         └─────────────┘
```

### Key Modules

1. **AuthModule**: JWT strategy and authentication guards
2. **ProxyModule**: Request proxying to microservices
3. **NotificationModule**: Notification processing logic
4. **RedisModule**: Global Redis client provider
5. **ThrottlerModule**: Rate limiting with Redis storage

---

## Features Implemented

### 1. JWT Authentication System

**Components:**
- `JwtStrategy`: Validates JWT tokens using Passport.js
- `JwtAuthGuard`: Global guard that protects routes (except public ones)
- `JwtHelper`: Manual token validation for Express middleware routes

**Features:**
- Automatic token extraction from `Authorization: Bearer <token>` header
- Token validation with configurable secret
- User context injection into request objects
- Public route bypass (signup/signin)

**Public Routes:**
- `/user/signup`
- `/user/signin`

**Protected Routes:**
- All other `/user/*` routes
- All `/template/*` routes
- All `/notifications/*` routes

### 2. Request Proxying

**ProxyMiddleware** handles routing requests to microservices:

- **Path-based routing**: Routes like `/user/*` → User Service, `/template/*` → Template Service
- **Header preservation**: Forwards all original headers
- **User context forwarding**: Adds `x-user-id` header for authenticated requests
- **Error handling**: Graceful error responses for proxy failures
- **Path transformation**: Strips prefix and forwards to target service

**Current Proxy Routes:**
```typescript
/user/* → http://localhost:3001
/template/* → http://localhost:3003
```

### 3. Notification Processing

**Complete notification workflow:**

1. **Request Validation**: Validates notification DTO
2. **User Preferences**: Fetches user preferences from user-service
3. **Channel Determination**: Determines available channels (EMAIL/PUSH)
4. **Template Fetching**: Retrieves templates from template-service
5. **Template Rendering**: Renders templates with provided data
6. **Queue Routing**: Routes messages to appropriate Redis queues
7. **Status Tracking**: Creates and stores notification status

### 4. Rate Limiting

- **Storage**: Redis-based throttling
- **Limit**: 100 requests per 60 seconds per IP
- **Storage Key**: `throttle:{identifier}`
- **TTL**: 60 seconds

### 5. Error Handling

**HttpExceptionFilter** provides consistent error responses:

```json
{
  "success": false,
  "message": "Error message",
  "error": "Error details",
  "data": {},
  "meta": {}
}
```

### 6. Response Interceptor

**ResponseInterceptor** wraps all successful responses:

```json
{
  "success": true,
  "message": "Operation successful",
  "data": { /* response data */ },
  "meta": {}
}
```

### 7. Logging

- Request/response logging via `LoggingInterceptor`
- Debug logs for authentication flow
- Proxy request/response logging
- Error logging with stack traces

---

## How It Works

### Request Flow

#### 1. Public Route (e.g., `/user/signin`)

```
Client Request
    ↓
API Gateway receives request
    ↓
JwtAuthGuard checks route → Public route detected → Allow
    ↓
ProxyMiddleware extracts route → /user/signin
    ↓
No JWT validation needed (public route)
    ↓
Proxy to User Service (http://localhost:3001/signin)
    ↓
User Service processes request
    ↓
Response returned to client
```

#### 2. Protected Route (e.g., `/user/profile`)

```
Client Request (with Bearer token)
    ↓
API Gateway receives request
    ↓
JwtAuthGuard checks route → Protected route → Validate token
    ↓
JwtHelper extracts token from Authorization header
    ↓
JwtHelper validates token signature and expiration
    ↓
If valid: Set req.user = { userId: "..." }
    ↓
ProxyMiddleware checks req.user → User found → Allow
    ↓
Add x-user-id header to proxy request
    ↓
Proxy to User Service with user context
    ↓
User Service processes request
    ↓
Response returned to client
```

#### 3. Notification Request

```
POST /notifications (with Bearer token)
    ↓
JwtAuthGuard validates token
    ↓
NotificationController receives request
    ↓
NotificationService.processNotification():
    1. Extract userId from token or request
    2. Fetch user preferences from user-service
    3. Determine channels (EMAIL/PUSH)
    4. For each channel:
       - Fetch template from template-service
       - Render template with data
       - Queue message to Redis (email:queue or push:queue)
    5. Create notification status in Redis
    ↓
Return notification ID and status
```

### JWT Token Validation Flow

**For NestJS Controllers:**
1. `JwtAuthGuard` intercepts request
2. Checks if route is public
3. If protected, calls `JwtStrategy.validate()`
4. Passport extracts token from header
5. Token verified with JWT_SECRET
6. Payload validated and `req.user` set

**For Express Middleware (Proxy Routes):**
1. `JwtHelper.extractTokenFromRequest()` extracts token
2. `JwtHelper.validateToken()` verifies signature
3. If valid, manually sets `req.user`
4. Proxy middleware checks `req.user` for authentication

### Proxy Middleware Flow

```typescript
Request → ProxyMiddleware.use()
    ↓
Sets req.proxy function
    ↓
Main.ts executes req.proxy()
    ↓
Checks authentication requirement
    ↓
If requires auth and no user → 401 Unauthorized
    ↓
Otherwise → Proxy to target service
    ↓
Preserve headers, add x-user-id if authenticated
    ↓
Forward request to target service
    ↓
Return response to client
```

---

## API Endpoints

### Notification Endpoints

#### Create Notification

```http
POST /notifications
Authorization: Bearer <JWT_TOKEN>
Content-Type: application/json

{
  "userId": "user-id-here",  // Optional, uses JWT user_id if not provided
  "event": "WELCOME_MESSAGE",
  "data": {
    "name": "John Doe",
    "orderId": "12345"
  },
  "channels": ["EMAIL", "PUSH"],  // Optional, uses user preferences if not provided
  "language": "en"  // Optional, defaults to "en"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Notification queued successfully",
  "data": {
    "notificationId": "550e8400-e29b-41d4-a716-446655440000",
    "status": "queued",
    "channels": ["EMAIL", "PUSH"]
  }
}
```

#### Get Notification Status

```http
GET /notifications/:id/status
Authorization: Bearer <JWT_TOKEN>
```

**Response:**
```json
{
  "success": true,
  "message": "Notification status retrieved successfully",
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "userId": "user-id",
    "event": "WELCOME_MESSAGE",
    "channels": ["EMAIL", "PUSH"],
    "status": "queued",
    "createdAt": "2025-01-01T00:00:00.000Z",
    "updatedAt": "2025-01-01T00:00:00.000Z"
  }
}
```

### Proxied Endpoints

#### User Service Routes

**Public Routes:**
- `POST /user/signup` - User registration
- `POST /user/signin` - User login

**Protected Routes (require JWT):**
- `GET /user/profile` - Get user profile
- `PUT /user/profile` - Update user profile
- `GET /user/preferences` - Get user preferences
- All other `/user/*` routes

#### Template Service Routes

**All routes require JWT authentication:**
- `GET /template/:id` - Get template by ID
- `POST /template` - Create template
- `PUT /template/:id` - Update template
- `DELETE /template/:id` - Delete template
- All other `/template/*` routes

---

## Testing Guide

### Prerequisites

1. **Start all services:**
   ```bash
   # Terminal 1: User Service
   cd services/user-service
   npm run start:dev

   # Terminal 2: Template Service
   cd services/template-service
   npm run start:dev

   # Terminal 3: API Gateway
   cd api-gateway
   npm run start:dev

   # Terminal 4: Redis
   redis-server
   ```

2. **Environment Variables:**
   Ensure all services have matching `JWT_SECRET` in their `.env` files.

### Test 1: User Signup (Public Route)

```bash
curl -X POST http://localhost:3000/user/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123",
    "name": "Test User"
  }'
```

**Expected:** 201 Created with user data and JWT token

### Test 2: User Signin (Public Route)

```bash
curl -X POST http://localhost:3000/user/signin \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123"
  }'
```

**Expected:** 200 OK with JWT token

**Save the token:**
```bash
export TOKEN="your-jwt-token-here"
```

### Test 3: Get User Profile (Protected Route)

```bash
curl -X GET http://localhost:3000/user/profile \
  -H "Authorization: Bearer $TOKEN"
```

**Expected:** 200 OK with user profile data

### Test 4: Create Template (Protected Route)

```bash
curl -X POST http://localhost:3000/template \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "event": "WELCOME_MESSAGE",
    "channel": "EMAIL",
    "subject": "Welcome {{name}}!",
    "html": "<h1>Welcome {{name}}!</h1><p>Your order ID is {{orderId}}</p>"
  }'
```

**Expected:** 201 Created with template data

### Test 5: Create Notification

```bash
curl -X POST http://localhost:3000/notifications \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "event": "WELCOME_MESSAGE",
    "data": {
      "name": "John Doe",
      "orderId": "12345"
    },
    "channels": ["EMAIL", "PUSH"],
    "language": "en"
  }'
```

**Expected:** 200 OK with notification ID and status

**Save the notification ID:**
```bash
export NOTIFICATION_ID="notification-id-from-response"
```

### Test 6: Get Notification Status

```bash
curl -X GET http://localhost:3000/notifications/$NOTIFICATION_ID/status \
  -H "Authorization: Bearer $TOKEN"
```

**Expected:** 200 OK with notification status

### Test 7: Rate Limiting

```bash
# Send 101 requests rapidly
for i in {1..101}; do
  curl -X GET http://localhost:3000/user/profile \
    -H "Authorization: Bearer $TOKEN"
  echo "Request $i"
done
```

**Expected:** First 100 requests succeed, 101st returns 429 Too Many Requests

### Test 8: Invalid Token

```bash
curl -X GET http://localhost:3000/user/profile \
  -H "Authorization: Bearer invalid-token"
```

**Expected:** 401 Unauthorized

### Test 9: Missing Token

```bash
curl -X GET http://localhost:3000/user/profile
```

**Expected:** 401 Unauthorized

### Using Postman

1. **Create Environment:**
   - Variable: `base_url` = `http://localhost:3000`
   - Variable: `token` = (set after signin)

2. **Collection Structure:**
   ```
   API Gateway
   ├── Auth
   │   ├── Signup (POST {{base_url}}/user/signup)
   │   └── Signin (POST {{base_url}}/user/signin)
   ├── User
   │   ├── Get Profile (GET {{base_url}}/user/profile)
   │   └── Update Profile (PUT {{base_url}}/user/profile)
   ├── Template
   │   ├── Create Template (POST {{base_url}}/template)
   │   └── Get Template (GET {{base_url}}/template/:id)
   └── Notifications
       ├── Create Notification (POST {{base_url}}/notifications)
       └── Get Status (GET {{base_url}}/notifications/:id/status)
   ```

3. **Set Authorization:**
   - Type: Bearer Token
   - Token: `{{token}}`

---

## Adding New Services

### Step 1: Update Environment Variables

Add the new service URL to `api-gateway/.env`:

```env
NEW_SERVICE_URL=http://localhost:3006
```

### Step 2: Update Config

Edit `src/config/config.ts`:

```typescript
export default () => ({
  // ... existing config
  newServiceUrl: process.env.NEW_SERVICE_URL,
});
```

### Step 3: Add Proxy Route

Edit `src/main.ts` and add to the `proxyRoutes` array:

```typescript
const proxyRoutes = [
  // ... existing routes
  {
    path: '/new-service',
    target: newServiceUrl,
    requireAuth: (req: Request) => {
      // Define which routes require authentication
      const originalUrl = req.originalUrl || req.url || '';
      const publicPaths = ['/new-service/public'];
      
      const isPublic = publicPaths.some((publicPath) => {
        return originalUrl === publicPath || originalUrl.startsWith(publicPath + '/');
      });
      
      return !isPublic; // Return true if route requires auth
    },
  },
];
```

### Step 4: Restart API Gateway

```bash
npm run start:dev
```

### Example: Adding a Payment Service

```typescript
// In src/main.ts
const proxyRoutes = [
  // ... existing routes
  {
    path: '/payment',
    target: paymentServiceUrl, // http://localhost:3006
    requireAuth: (req: Request) => {
      // All payment routes require authentication
      return true;
    },
  },
];
```

Now all requests to `/payment/*` will be proxied to the payment service at `http://localhost:3006`.

### Advanced: Custom Authentication Logic

You can implement custom authentication logic per service:

```typescript
{
  path: '/admin',
  target: adminServiceUrl,
  requireAuth: (req: Request) => {
    // Check if user has admin role
    const userReq = req as unknown as UserRequest;
    if (userReq.user?.role === 'admin') {
      return true;
    }
    return false; // Public route for non-admins
  },
}
```

### Advanced: Service-Specific Headers

To add custom headers for a specific service, modify `ProxyMiddleware`:

```typescript
// In proxy.middleware.ts, in proxyReqOptDecorator
if (targetUrl.includes('payment-service')) {
  proxyReqOpts.headers['x-service-version'] = 'v2';
}
```

---

## Configuration

### Environment Variables

Create `api-gateway/.env`:

```env
# Server Configuration
PORT=3000

# Redis Configuration
REDIS_URL=redis://localhost:6379

# Service URLs
API_GATEWAY_URL=http://localhost:3000
USER_SERVICE_URL=http://localhost:3001
ORCHESTRATOR_SERVICE_URL=http://localhost:3002
TEMPLATE_SERVICE_URL=http://localhost:3003
EMAIL_SERVICE_URL=http://localhost:3004
PUSH_SERVICE_URL=http://localhost:3005

# JWT Configuration
JWT_SECRET=your-secret-key-here  # MUST match user-service JWT_SECRET
```

### Rate Limiting Configuration

Edit `src/app.module.ts`:

```typescript
ThrottlerModule.forRootAsync({
  // ...
  useFactory: (storage: CustomRedisStorageService) => ({
    throttlers: [
      {
        ttl: 60,        // Time window in seconds
        limit: 100,     // Max requests per window
      },
    ],
    storage,
  }),
}),
```

### CORS Configuration

Edit `src/main.ts`:

```typescript
app.enableCors({
  origin: 'http://localhost:3000',  // Allowed origins
  credentials: true,
});
```

---

## Troubleshooting

### Issue: "Invalid signature" when validating JWT

**Cause:** JWT_SECRET mismatch between API Gateway and user-service

**Solution:**
1. Check `JWT_SECRET` in both services' `.env` files
2. Ensure they are **exactly** the same
3. Restart both services

### Issue: "Unauthorized" for public routes

**Cause:** JwtAuthGuard is blocking public routes

**Solution:**
1. Check that public routes are listed in `jwt-auth.guard.ts`
2. Verify route paths match exactly (case-sensitive)
3. Check logs for route matching debug messages

### Issue: "Cannot POST /user/signin"

**Cause:** Proxy path resolution issue

**Solution:**
1. Check that `proxyReqPathResolver` in `proxy.middleware.ts` is correct
2. Verify target service is running
3. Check network connectivity

### Issue: "Proxy execution error"

**Cause:** Target service is down or unreachable

**Solution:**
1. Verify target service is running
2. Check service URL in `.env`
3. Test direct connection to target service
4. Check firewall/network settings

### Issue: Rate limiting not working

**Cause:** Redis connection issue

**Solution:**
1. Verify Redis is running: `redis-cli ping`
2. Check `REDIS_URL` in `.env`
3. Check Redis connection logs

### Issue: Notification status not found

**Cause:** Status expired or never created

**Solution:**
1. Check Redis: `redis-cli GET notification:status:{id}`
2. Verify notification was created successfully
3. Check TTL: `redis-cli TTL notification:status:{id}`

### Debug Mode

Enable debug logging by setting log level:

```typescript
// In src/main.ts
const app = await NestFactory.create(AppModule, {
  logger: ['error', 'warn', 'log', 'debug', 'verbose'],
});
```

### Check Logs

All logs are prefixed with module names:
- `[JWT]` - JWT validation logs
- `[ProxyMiddleware]` - Proxy routing logs
- `[NotificationService]` - Notification processing logs
- `[JwtAuthGuard]` - Authentication guard logs

---

## File Structure

```
api-gateway/
├── src/
│   ├── auth/                    # Authentication module
│   │   ├── auth.module.ts
│   │   ├── jwt-auth.guard.ts   # Global JWT guard
│   │   └── jwt.strategy.ts     # Passport JWT strategy
│   ├── common/                  # Shared utilities
│   │   ├── interceptors/
│   │   │   └── response.interceptors.ts  # Response formatting
│   │   ├── jwt-helper.ts       # Manual JWT validation
│   │   └── redis.module.ts     # Global Redis client
│   ├── config/
│   │   └── config.ts           # Environment config
│   ├── middleware/
│   │   ├── logging.interceptor.ts  # Request logging
│   │   ├── proxy.middleware.ts    # Request proxying
│   │   └── proxy.module.ts
│   ├── notification/            # Notification module
│   │   ├── dto/
│   │   │   └── create-notification.dto.ts
│   │   ├── notification.controller.ts
│   │   ├── notification.module.ts
│   │   └── notification.service.ts
│   ├── throttler/              # Rate limiting
│   │   ├── redis-storage.service.ts
│   │   └── throttler-storage.module.ts
│   ├── app.module.ts           # Root module
│   └── main.ts                 # Application entry point
├── .env                        # Environment variables
├── package.json
└── NOTIFICATION_API.md         # This file
```

---

## Best Practices

1. **JWT_SECRET**: Use strong, unique secrets in production. Never commit secrets to version control.

2. **Error Handling**: Always check `res.headersSent` before sending error responses.

3. **Logging**: Use appropriate log levels (debug, info, warn, error).

4. **Rate Limiting**: Adjust limits based on your service capacity and requirements.

5. **CORS**: Configure CORS properly for production environments.

6. **Health Checks**: Consider adding `/health` endpoint for monitoring.

7. **Service Discovery**: In production, use service discovery instead of hardcoded URLs.

---

## Next Steps

- [ ] Add health check endpoints
- [ ] Implement request/response caching
- [ ] Add metrics and monitoring
- [ ] Implement circuit breaker pattern
- [ ] Add API versioning
- [ ] Implement request tracing
- [ ] Add Swagger/OpenAPI documentation

---

## Support

For issues or questions:
1. Check logs for error messages
2. Verify environment variables
3. Test services individually
4. Check network connectivity
5. Review this documentation

---

**Last Updated:** January 2025
**Version:** 1.0.0
