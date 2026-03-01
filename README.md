# Doowork - Project Management Microservices

ระบบจัดการโปรเจคแบบ Microservices สร้างด้วย Go (Golang)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         API Gateway                              │
│                    (Optional - Future)                          │
└─────────────────────────────────────────────────────────────────┘
        │               │               │               │
        ▼               ▼               ▼               ▼
┌───────────────┐ ┌───────────────┐ ┌───────────────┐ ┌───────────────┐
│  User Service │ │Project Service│ │  Task Service │ │ Notification  │
│   Port: 8081  │ │  Port: 8082   │ │  Port: 8083   │ │   Port: 8084  │
└───────────────┘ └───────────────┘ └───────────────┘ └───────────────┘
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

## Tech Stack

- **Language**: Go 1.21
- **Web Framework**: Gin
- **ORM**: GORM
- **Database**: PostgreSQL 15
- **Authentication**: JWT
- **Container**: Docker & Docker Compose
- **Scheduler**: robfig/cron (for notifications)

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
