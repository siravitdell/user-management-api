# User Management API

A user management service written in Go. It offers REST and gRPC APIs, stores users in MongoDB, authenticates with JWT (HS256), and is structured as a hexagonal (ports and adapters) architecture.

## Features

- **Users:** create, get by ID, list, update name/email, delete. Email is unique, the password is stored as a bcrypt hash, and the ID and `created_at` are generated.
- **Auth:** register and log in to get a JWT signed with HS256. The `RequireAuth` middleware protects every `/api/v1/users` route.
- **MongoDB:** uses the official Go driver (`go.mongodb.org/mongo-driver/v2`).
- **Logging:** every request is logged with its method, path, status and duration.
- **Background job:** a goroutine logs the total number of users every 10 seconds.
- **Tests:** unit tests use the standard `testing` package. MongoDB is mocked through the `port.UserRepository` interface.
- **Bonus:** Docker and docker-compose, interface abstraction, input validation, graceful shutdown, gRPC `CreateUser`/`GetUser` with token metadata, and hexagonal architecture.

## Quick start

Requirements: Docker with Docker Compose, **or** Go 1.27+ plus MongoDB.

**1. Create `.env`** and set `JWT_SECRET` to a random value of at least 32 bytes. The server will not start without it.

```bash
cp .env.example .env
openssl rand -hex 32        # paste the output into JWT_SECRET=
```

**2a. Run with Docker (recommended).** This starts the REST API on `:8080`, gRPC on `:50051` and MongoDB on `:27017`.

```bash
docker compose up --build -d
docker compose logs -f api
docker compose down
```

**2b. Or run locally with Go.** `go run` does not read `.env`, so load it into the shell first.

```bash
docker compose up -d mongo
set -a; source .env; set +a         # PowerShell: see the note below
go run ./cmd/server
```

<details>
<summary>PowerShell: load <code>.env</code></summary>

```powershell
Get-Content .env | Where-Object { $_ -match '^[A-Z_]+=' } | ForEach-Object {
  $k, $v = $_ -split '=', 2; Set-Item "env:$k" $v
}
```

</details>

### Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `JWT_SECRET` | **required** | HMAC-SHA256 key, at least 32 bytes |
| `JWT_ISSUER` | `user-management-api` | `iss` claim. The server sets it on new tokens and checks it on verify. |
| `JWT_TTL` | `1h` | Token lifetime |
| `HTTP_ADDR` / `GRPC_ADDR` | `:8080` / `:50051` | Listen addresses |
| `MONGO_URI` | `mongodb://localhost:27017` | docker-compose overrides it with `mongodb://mongo:27017` |
| `MONGO_DATABASE` | `user_management` | Database name |
| `USER_COUNT_INTERVAL` | `10s` | Interval of the user-count log |

## JWT guide

1. Register: `POST /api/v1/auth/register`
2. Log in: `POST /api/v1/auth/login`. The response contains `access_token`.
3. Send the header `Authorization: Bearer <access_token>` on every protected request. gRPC uses the same token in the `authorization` metadata.

```bash
curl -s -X POST localhost:8080/api/v1/auth/register -H 'Content-Type: application/json' \
  -d '{"name":"Alice","email":"alice@example.com","password":"password123"}'

TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123"}' | sed -E 's/.*"access_token":"([^"]+)".*/\1/')

curl -s localhost:8080/api/v1/users -H "Authorization: Bearer $TOKEN"
```

The decoded payload of a token looks like this:

```json
{ "sub": "<user id>", "email": "alice@example.com", "iss": "user-management-api",
  "iat": 1794936054, "nbf": 1794936054, "exp": 1794939654, "jti": "10d1075f..." }
```

The server accepts a token only if all of these hold:

- the algorithm is HS256, which rules out `alg: none` and algorithm confusion
- the signature matches `JWT_SECRET`
- `exp` is present and still in the future (5 s leeway)
- `iss` matches `JWT_ISSUER`

There is no refresh token. When a token expires, log in again.

## REST API

| Method | Path | Auth | Success |
| --- | --- | --- | --- |
| POST | `/api/v1/auth/register` | — | 201 user |
| POST | `/api/v1/auth/login` | — | 200 token |
| POST | `/api/v1/users` | JWT | 201 user |
| GET | `/api/v1/users` | JWT | 200 array of users |
| GET | `/api/v1/users/{id}` | JWT | 200 user |
| PATCH | `/api/v1/users/{id}` | JWT | 200 user (send `name` and/or `email`) |
| DELETE | `/api/v1/users/{id}` | JWT | 204 |

Validation rules:

- `name`: required, at most 100 characters
- `email`: required, a valid address, at most 254 characters. It is trimmed and lower-cased.
- `password`: 8–72 bytes

### Sample requests and responses

These responses were captured from a running instance.

```bash
# Create (same body for /auth/register)
curl -s -X POST localhost:8080/api/v1/users -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"name":"Bob","email":"bob@example.com","password":"password123"}'
```
```json
{"id":"6afc8cf61389c013d865a686","name":"Bob","email":"bob@example.com","created_at":"2026-11-17T17:20:54.8Z"}
```

```bash
# Login
curl -s -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123"}'
```
```json
{"access_token":"eyJhbGciOiJIUzI1NiIs...","token_type":"Bearer","expires_in":3599,"expires_at":"2026-11-17T18:20:54.59Z"}
```

```bash
# Get / List
curl -s localhost:8080/api/v1/users/6afc8cf61389c013d865a686 -H "Authorization: Bearer $TOKEN"
curl -s localhost:8080/api/v1/users -H "Authorization: Bearer $TOKEN"
```
```json
[{"id":"6afc8cf61389c013d865a684","name":"Alice","email":"alice@example.com","created_at":"2026-11-17T17:20:54.298Z"},
 {"id":"6afc8cf61389c013d865a686","name":"Bob","email":"bob@example.com","created_at":"2026-11-17T17:20:54.8Z"}]
```

```bash
# Update
curl -s -X PATCH localhost:8080/api/v1/users/6afc8cf61389c013d865a686 -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"name":"Bob Smith"}'
```
```json
{"id":"6afc8cf61389c013d865a686","name":"Bob Smith","email":"bob@example.com","created_at":"2026-11-17T17:20:54.8Z"}
```

```bash
# Delete: 204 No Content
curl -s -X DELETE localhost:8080/api/v1/users/6afc8cf61389c013d865a686 -H "Authorization: Bearer $TOKEN"
```

### Errors

Every error has the form `{"error":{"code","message","details?"}}`. For example:

```json
{"error":{"code":"validation_failed","message":"request validation failed","details":[
  {"field":"name","message":"is required"},{"field":"email","message":"must be a valid email address"}]}}
```

| HTTP | `code` | When |
| --- | --- | --- |
| 400 | `invalid_json` | The body is not a single valid JSON object. This includes unknown fields (such as `password` in PATCH) and bodies over 1 MiB. |
| 400 | `validation_failed` | A field is invalid. `details` lists every failing field. |
| 401 | `unauthorized` | The token is missing, malformed, invalid or expired |
| 401 | `invalid_credentials` | Wrong email or password |
| 404 | `not_found` | The user does not exist |
| 409 | `email_taken` | The email is already used |
| 500 | `internal_error` | Unexpected error. Details are logged on the server, never returned to the client. |

## gRPC API

[user.proto](api/proto/user/v1/user.proto) defines `CreateUser` and `GetUser`. Every call requires `authorization: Bearer <token>` metadata. Try it with the bundled client:

```bash
CID=$(go run ./cmd/grpc-client -token "$TOKEN" create -name Carol -email carol@example.com -password password123 \
  | grep '"id"' | cut -d'"' -f4)
go run ./cmd/grpc-client -token "$TOKEN" get -id "$CID"
```
```json
{"user":{"id":"6afc8cf71389c013d865a687","name":"Carol","email":"carol@example.com","createdAt":"2026-11-17T17:20:55.706Z"}}
```

Domain errors are mapped to gRPC codes: `InvalidArgument`, `NotFound`, `AlreadyExists`, `Unauthenticated` or `Internal`.

## Testing

```bash
go test ./...
go test -cover ./internal/...
MONGO_TEST_URI=mongodb://localhost:27017 go test -count=1 ./internal/adapter/outbound/mongodb/...   # real MongoDB
```

The services are tested with a mocked repository. The HTTP and gRPC layers are tested with an in-memory fake repository, and gRPC runs over `bufconn`. JWT tests cover expired, tampered, `alg: none` and wrong-issuer tokens.

## Project structure

```text
cmd/server/            wiring, goroutines, graceful shutdown
cmd/grpc-client/       example gRPC client
api/proto/, gen/       gRPC contract and generated code
internal/config/       environment variables
internal/core/         business logic (imports no adapters)
  domain/              User entity, validation, errors
  port/                interfaces
  service/             UserService, AuthService
internal/adapter/
  inbound/             httpapi (REST), grpcapi, scheduler   -> call the core
  outbound/            mongodb, security (bcrypt, JWT)      <- used by the core
```

## Design decisions and assumptions

- **Hexagonal architecture.** REST, gRPC and the scheduler call the same `port.UserService`, so the business rules exist only once. MongoDB documents (`userDocument`) are kept separate from the domain model.
- **Email uniqueness is enforced by a MongoDB unique index,** not by a check-then-insert in code. This is race-free. Emails are lower-cased first, so `Alice@x.com` and `alice@x.com` count as the same address.
- **Login is timing-safe.** For an unknown email the server still compares against a dummy bcrypt hash, so response time does not reveal which emails exist. A wrong email and a wrong password return the same error.
- **Errors never leak internals.** Unexpected errors are logged on the server and the client gets a generic 500. All JSON decoding errors return the same `invalid_json` error.
- **IDs are MongoDB ObjectIDs** in hex. A malformed ID returns 404, because such a user cannot exist.
- **List returns all users** in creation order, with no pagination, because the assignment asks to "list all users".
- **Registration and create user do the same thing.** `/auth/register` is public, while `POST /users` requires a token.
- **There are no roles.** Any authenticated user can manage any user, because the assignment defines no authorization rules.
- **The password cannot be changed with PATCH.** The assignment limits updates to name and email.
- **Tokens are stateless.** There is no logout or revocation. Keep `JWT_TTL` short.
- **Graceful shutdown.** On SIGINT/SIGTERM the server waits up to 10 seconds for in-flight HTTP and gRPC requests, then stops the background job and disconnects from MongoDB.
