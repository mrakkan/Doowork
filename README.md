# Doowork - Project Management Microservices

ระบบจัดการโปรเจคแบบ Microservices สร้างด้วย Go

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         API Gateway                              │
└─────────────────────────────────────────────────────────────────┘
    │               │               │               │
    ▼               ▼               ▼               ▼
┌───────────────┐ ┌───────────────┐ ┌───────────────┐ ┌───────────────┐
│  User Service │ │Project Service│ │  Task Service │ │ Notification  │
│   Port: 8081  │ │  Port: 8082   │ │  Port: 8083   │ │   Port: 8084  │
└───────────────┘ └───────────────┘ └───────────────┘ └───────────────┘
            │               │               ▲
            └───────┬───────┴───────────────┘
                ▼
            ┌───────────────┐
            │   RabbitMQ    │
            │ Port: 5672    │
            │ Mgmt: 15672   │
            └───────────────┘
        │               │               │               │
        ▼               ▼               ▼               ▼
┌───────────────┐ ┌───────────────┐ ┌───────────────┐ ┌───────────────┐
│   PostgreSQL  │ │   PostgreSQL  │ │   PostgreSQL  │ │   PostgreSQL  │
│   Port: 5432  │ │   Port: 5433  │ │   Port: 5434  │ │   Port: 5435  │
└───────────────┘ └───────────────┘ └───────────────┘ └───────────────┘
```

## Services

### 1. User Service (Port 8081)
- **Register** - สมัครสมาชิกใหม่
- **Login** - เข้าสู่ระบบ (JWT Token)
- **Logout** - ออกจากระบบ
- **Add Member** - เพิ่มสมาชิกในโปรเจค
- **Edit Member** - แก้ไขข้อมูลสมาชิก
- **Delete Member** - ลบสมาชิก

### 2. Project Service (Port 8082)
- **Create Project** - สร้างโปรเจคใหม่
- **Update Project** - แก้ไขโปรเจค
- **Delete Project** - ลบโปรเจค
- **Show Project Status** - แสดงสถานะโปรเจค
- **Show Project List** - แสดงรายการโปรเจค
- **Project Members** - จัดการสมาชิกในโปรเจค

### 3. Task Service (Port 8083)
- **Create Task** - สร้าง Task ใหม่
- **Update Task** - แก้ไข Task
- **Delete Task** - ลบ Task
- **Assign Task** - มอบหมาย Task
- **Show Task Status** - แสดงสถานะ Task
- **Show Task** - แสดงรายละเอียด Task
- **Calculate Time** - คำนวณเวลาที่ใช้
- **Calculate Price** - คำนวณราคา/ต้นทุน

### 4. Notification Service (Port 8084)
- **Send Notification** - ส่ง Notification
- **Schedule Notification** - ตั้งเวลาส่ง Notification
- **Allow Notification** - เปิด/ปิด Notification
- **Event Consumer** - รับ Event จาก RabbitMQ (`project.*`, `task.*`) แล้วสร้าง notification อัตโนมัติ

## Event-Driven Communication

- `project-service` publish: `project.created`, `project.member_added`
- `task-service` publish: `task.created`, `task.assigned`
- `notification-service` consume events ผ่าน RabbitMQ แล้วบันทึกลง `notifications`
- ใช้ Circuit Breaker สำหรับการเรียกข้าม service

## Quick Start

### Prerequisites
- Docker & Docker Compose
- Go 1.21+ (for local development)
- Postman (for API testing)

### Run with Docker Compose

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop all services
docker-compose down

# Stop and remove volumes
docker-compose down -v
```

### Service Ports

| Service | Port |
|---------|------|
| User Service | 8081 |
| Project Service | 8082 |
| Task Service | 8083 |
| Notification Service | 8084 |
| Prometheus | 9090 |
| RabbitMQ AMQP | 5672 |
| RabbitMQ Management | 15672 |
| User DB | 5432 |
| Project DB | 5433 |
| Task DB | 5434 |
| Notification DB | 5435 |

## API Testing with Postman

1. Import `Doowork.postman_collection.json` into Postman
2. Start the services with `docker-compose up -d`
3. Test the APIs:

### Authentication Flow

```
1. POST /api/auth/register - Register a new user
2. POST /api/auth/login    - Login and get JWT token
3. Use the token in Authorization header: "Bearer <token>"
```

### Example Usage

#### Register User
```http
POST http://localhost:8081/api/auth/register
Content-Type: application/json

{
    "email": "test@example.com",
    "password": "password123",
    "name": "Test User"
}
```

#### Login
```http
POST http://localhost:8081/api/auth/login
Content-Type: application/json

{
    "email": "test@example.com",
    "password": "password123"
}
```

#### Create Project
```http
POST http://localhost:8082/api/projects
Authorization: Bearer <token>
Content-Type: application/json

{
    "name": "My Project",
    "description": "Project description",
    "budget": 50000
}
```

#### Create Task
```http
POST http://localhost:8083/api/tasks
Authorization: Bearer <token>
Content-Type: application/json

{
    "title": "Implement login",
    "project_id": 1,
    "priority": "high",
    "estimated_hours": 8,
    "hourly_rate": 500
}
```

#### Send Notification
```http
POST http://localhost:8084/api/notifications
Authorization: Bearer <token>
Content-Type: application/json

{
    "user_id": 1,
    "title": "Task Assigned",
    "message": "You have been assigned a new task",
    "type": "task"
}
```

## Test Cases (5 ต่อ Service)

### User Service
- **TC-USER-01 Register Success**: `POST /api/auth/register` ควรได้ `201`
    - Raw JSON Body:
        ```json
        {
            "email": "user01@example.com",
            "password": "password123",
            "name": "User 01"
        }
        ```
- **TC-USER-02 Register Duplicate Email**: `POST /api/auth/register` (email เดิม) ควรได้ `409`
    - Raw JSON Body:
        ```json
        {
            "email": "user01@example.com",
            "password": "password123",
            "name": "User Duplicate"
        }
        ```
- **TC-USER-03 Login Success**: `POST /api/auth/login` ควรได้ `200` และมี `token`
    - Raw JSON Body:
        ```json
        {
            "email": "user01@example.com",
            "password": "password123"
        }
        ```
- **TC-USER-04 Login Invalid Password**: `POST /api/auth/login` ควรได้ `401`
    - Raw JSON Body:
        ```json
        {
            "email": "user01@example.com",
            "password": "wrong-password"
        }
        ```
- **TC-USER-05 Get Current User**: `GET /api/users/me` ควรได้ `200`
    - Raw JSON Body: `(none)`

### Project Service
- **TC-PROJ-01 Create Project**: `POST /api/projects` ควรได้ `201`
    - Raw JSON Body:
        ```json
        {
            "name": "Project Alpha",
            "description": "First project",
            "budget": 50000
        }
        ```
- **TC-PROJ-02 List Projects**: `GET /api/projects` ควรได้ `200`
    - Raw JSON Body: `(none)`
- **TC-PROJ-03 Get Project By ID**: `GET /api/projects/:id` ควรได้ `200`
    - Raw JSON Body: `(none)`
- **TC-PROJ-04 Add Project Member**: `POST /api/projects/:id/members` ควรได้ `201`
    - Raw JSON Body:
        ```json
        {
            "user_id": 2,
            "role": "member"
        }
        ```
- **TC-PROJ-05 Add Duplicate Member**: `POST /api/projects/:id/members` (user เดิม) ควรได้ `409`
    - Raw JSON Body:
        ```json
        {
            "user_id": 2,
            "role": "member"
        }
        ```

### Task Service
- **TC-TASK-01 Create Task**: `POST /api/tasks` ควรได้ `201`
    - Raw JSON Body:
        ```json
        {
            "title": "Implement API",
            "description": "Build project API",
            "project_id": 1,
            "priority": "high",
            "estimated_hours": 8,
            "hourly_rate": 500
        }
        ```
- **TC-TASK-02 Create Task Invalid Project**: `POST /api/tasks` ควรได้ `400`
    - Raw JSON Body:
        ```json
        {
            "title": "Invalid Project Task",
            "description": "Should fail",
            "project_id": 99999,
            "priority": "medium",
            "estimated_hours": 3,
            "hourly_rate": 300
        }
        ```
- **TC-TASK-03 Assign Task Success**: `POST /api/tasks/:id/assign` ควรได้ `201`
    - Raw JSON Body:
        ```json
        {
            "user_id": 2,
            "role": "assignee"
        }
        ```
- **TC-TASK-04 Assign Task Duplicate**: `POST /api/tasks/:id/assign` (user เดิม) ควรได้ `409`
    - Raw JSON Body:
        ```json
        {
            "user_id": 2,
            "role": "assignee"
        }
        ```
- **TC-TASK-05 Update Task Status**: `PUT /api/tasks/:id/status` ควรได้ `200`
    - Raw JSON Body:
        ```json
        {
            "status": "in_progress"
        }
        ```

### Notification Service
- **TC-NOTI-01 Send Notification Success**: `POST /api/notifications` ควรได้ `201`
    - Raw JSON Body:
        ```json
        {
            "user_id": 1,
            "title": "Task Assigned",
            "message": "You have been assigned a new task",
            "type": "task"
        }
        ```
- **TC-NOTI-02 Send Notification User Not Found**: `POST /api/notifications` ควรได้ `400`
    - Raw JSON Body:
        ```json
        {
            "user_id": 99999,
            "title": "Invalid User",
            "message": "This should fail",
            "type": "task"
        }
        ```
- **TC-NOTI-03 Get Notifications**: `GET /api/notifications` ควรได้ `200`
    - Raw JSON Body: `(none)`
- **TC-NOTI-04 Mark As Read**: `PUT /api/notifications/:id/read` ควรได้ `200`
    - Raw JSON Body: `(none)`
- **TC-NOTI-05 Update Preferences**: `PUT /api/notifications/preferences` ควรได้ `200`
    - Raw JSON Body:
        ```json
        {
            "allow_all": true,
            "allow_task": true,
            "allow_project": true,
            "email_enabled": false,
            "push_enabled": true
        }
        ```

### Gateway Service
- **TC-GW-01 Route Auth Request**: `POST /api/auth/login` ผ่าน `:8000` ควร route ไป user-service
    - Raw JSON Body:
        ```json
        {
            "email": "user01@example.com",
            "password": "password123"
        }
        ```
- **TC-GW-02 Route Project Request**: `GET /api/projects` ผ่าน `:8000` ควร route ไป project-service
    - Raw JSON Body: `(none)`
- **TC-GW-03 Route Task Price Request**: `GET /api/projects/:id/calculate-price` ผ่าน `:8000` ควร route ไป task-service
    - Raw JSON Body: `(none)`
- **TC-GW-04 Health Check**: `GET /health` ผ่าน `:8000` ควรได้ `200`
    - Raw JSON Body: `(none)`
- **TC-GW-05 Unknown Path**: เรียก `GET /unknown` ผ่าน `:8000` ควรได้ `404`
    - Raw JSON Body: `(none)`

## Tech Stack

- **Language**: Go 1.21
- **Web Framework**: Gin
- **ORM**: GORM
- **Database**: PostgreSQL 15
- **Authentication**: JWT
- **Container**: Docker & Docker Compose
- **Monitoring**: Prometheus
- **Message Broker**: RabbitMQ
- **Resilience**: Circuit Breaker (gobreaker)
- **Scheduler**: robfig/cron (for notifications)

## Monitoring with Prometheus

- Open Prometheus at `http://localhost:9090`
- Check service health in **Status > Targets**
- Example queries:
    - `doowork_http_requests_total`
    - `rate(doowork_http_requests_total[1m])`
    - `histogram_quantile(0.95, sum(rate(doowork_http_request_duration_seconds_bucket[5m])) by (le, service, path))`

## Project Structure

```
Doowork/
├── docker-compose.yml
├── README.md
├── Doowork.postman_collection.json
│
├── user-service/
│   ├── Dockerfile
│   ├── go.mod
│   ├── main.go
│   ├── handlers/
│   │   └── handler.go
│   ├── middleware/
│   │   └── auth.go
│   └── models/
│       └── user.go
│
├── project-service/
│   ├── Dockerfile
│   ├── go.mod
│   ├── main.go
│   ├── handlers/
│   │   └── handler.go
│   ├── middleware/
│   │   └── auth.go
│   └── models/
│       └── project.go
│
├── task-service/
│   ├── Dockerfile
│   ├── go.mod
│   ├── main.go
│   ├── handlers/
│   │   └── handler.go
│   ├── middleware/
│   │   └── auth.go
│   └── models/
│       └── task.go
│
└── notification-service/
    ├── Dockerfile
    ├── go.mod
    ├── main.go
    ├── handlers/
    │   └── handler.go
    ├── middleware/
    │   └── auth.go
    ├── models/
    │   └── notification.go
    └── scheduler/
        └── scheduler.go
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| DATABASE_URL | PostgreSQL connection string | - |
| JWT_SECRET | Secret key for JWT | your-secret-key-change-in-production |
| PORT | Service port | varies per service |
| RABBITMQ_URL | RabbitMQ connection string | amqp://guest:guest@rabbitmq:5672/ |
| TZ | Timezone | Asia/Bangkok |

## Task Status Values

| Status | Description |
|--------|-------------|
| todo | ยังไม่เริ่ม |
| in_progress | กำลังทำ |
| review | รอตรวจสอบ |
| done | เสร็จแล้ว |
| cancelled | ยกเลิก |

## Project Status Values

| Status | Description |
|--------|-------------|
| planning | วางแผน |
| in_progress | กำลังดำเนินการ |
| completed | เสร็จสิ้น |
| on_hold | พักไว้ |
| cancelled | ยกเลิก |

## Priority Values

| Priority | Description |
|----------|-------------|
| low | ต่ำ |
| medium | ปานกลาง |
| high | สูง |
| urgent | เร่งด่วน |

## License

MIT
