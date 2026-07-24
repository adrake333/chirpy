# Chirpy

A lightweight HTTP server that mimics a simplified social media backend, built in Go as a learning project.

1. It's a REST API
2. It handles user creation, authentication, and "chirps" (posts)
3. It uses JWT's, webhooks, and a database

## Setup

1. Clone the repo
2. Copy `.env.example` to `.env` and fill in your values
3. Run migrations: `goose up`
4. Start the server: `go run .`

### Usage

### Create a user

```bash
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "password": "hunter2"}'


### Logging in

```markdown
### Log in

```bash
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "password": "hunter2"}'

### Endpoints
| Method | Endpoint              | Description          |
|--------|-----------------------|----------------------|
| POST   | /api/users            | Create a new user    |
| POST   | /api/chirps           | Create a new chirp   |
| GET    | /api/chirps           | Get all chirps       |
| GET    | /api/metrics          | Check metrics        |
| POST   | /api/reset            | Reset DB (admin)     |
| POST   | /api/login            | Logs in              |
| GET    | /api/chirps/{chirpID} | Get one chirp        |
| PUT    | /api/users            | Update credentials   |
| DELETE | /api/chirps/{chirpID} | Delete chirp         |
