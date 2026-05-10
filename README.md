# Super App Chonburi — Mobile API

> **Go (Fiber) REST API** สำหรับจัดการ User ฝั่ง Mobile Application  
> เป็น service แยกต่างหากจาก `super-app-chonburi-go` (Admin Backend)  
> ใช้ **ฐานข้อมูลเดียวกัน** แต่จัดการตารางของ User จริง (ประชาชน) โดยเฉพาะ

---

## 📌 ภาพรวมระบบ

| Service | Repo | Port | ผู้ใช้งาน |
|---|---|---|---|
| Admin Backend | `super-app-chonburi-go` | `8080` | Admin / เจ้าหน้าที่ |
| **Mobile API** | `super-app-chonburi-go-mobile` | `8081` | **ประชาชน (Mobile App)** |
| Frontend Dashboard | `super-app-chonburi` | `3000` | Admin |

> ทั้งสอง Backend ใช้ **PostgreSQL Database เดียวกัน** (`uat_chonburi`)  
> แต่ Mobile API จะ **ไม่แตะตาราง `admin_users`** และจัดการตารางของตัวเองแทน

---

## 🏗️ โครงสร้าง Project

```
super-app-chonburi-go-mobile/
│
├── cmd/
│   └── server/
│       └── main.go             # Entry point — Fiber app setup
│
├── config/
│   └── config.go               # โหลด ENV variables
│
├── internal/                   # Business logic (ยังว่าง — รอ plan)
│   ├── domain/                 # Entities + Repository/UseCase Interfaces
│   ├── repository/             # Database layer (GORM)
│   ├── usecase/                # Business logic
│   └── delivery/
│       └── http/               # HTTP Handlers (Fiber)
│
├── pkg/
│   └── database/
│       └── postgres.go         # เชื่อมต่อ PostgreSQL ด้วย GORM
│
├── .env                        # Environment variables (ไม่ commit)
├── .env.example                # ตัวอย่าง ENV
├── go.mod / go.sum             # Go dependencies
└── package.json                # Dev scripts (nodemon)
```

### Clean Architecture Flow

```
HTTP Request
    │
    ▼
[delivery/http]  ←  Handler รับ request, validate input
    │
    ▼
[usecase]        ←  Business logic
    │
    ▼
[repository]     ←  เข้าถึง Database (GORM)
    │
    ▼
[domain]         ←  Entities & Interfaces (ไม่ขึ้นกับชั้นอื่น)
```

---

## 🔐 Authentication Strategy

> **ยังไม่ได้ implement** — รอการวางแผน

แผนคือใช้ **OAuth 2.0** แทน email/password login:
- Google OAuth
- Facebook OAuth
- (อนาคต) LINE OAuth

เมื่อ login สำเร็จจะออก JWT สำหรับเรียก API ที่ต้องการ auth

> ⚠️ JWT ของ Mobile API **แยกจาก** JWT ของ Admin Backend โดยสิ้นเชิง

---

## 🗄️ Database

- **Engine**: PostgreSQL
- **ORM**: GORM
- **ตารางที่จะสร้าง** (ของ Mobile เท่านั้น):

| ตาราง | คำอธิบาย |
|---|---|
| `users` | ข้อมูล User (ประชาชน) |
| `user_oauth_accounts` | เชื่อม OAuth provider (Google, Facebook) |
| `user_refresh_tokens` | Refresh token สำหรับ JWT rotation |
| `user_devices` | FCM Token สำหรับ Push Notification |

> **ไม่มี** `admin_users`, `admin_roles`, `departments` ในส่วนนี้  
> ตารางเหล่านั้นจัดการโดย `super-app-chonburi-go`

---

## ⚙️ Environment Variables

คัดลอก `.env.example` แล้วแก้ไขค่า:

```bash
cp .env.example .env
```

| Variable | คำอธิบาย | ค่าเริ่มต้น |
|---|---|---|
| `DB_DSN` | PostgreSQL connection string | **(required)** |
| `PORT` | Port ที่ server ฟัง | `8081` |
| `JWT_SECRET` | Secret key สำหรับ sign JWT | **(required)** |
| `GOOGLE_CLIENT_ID` | Google OAuth Client ID | — |
| `GOOGLE_CLIENT_SECRET` | Google OAuth Client Secret | — |
| `FACEBOOK_APP_ID` | Facebook OAuth App ID | — |
| `FACEBOOK_APP_SECRET` | Facebook OAuth App Secret | — |

---

## 🚀 การรัน Project

### Prerequisites
- Go 1.21+
- Node.js (สำหรับ nodemon dev script)
- PostgreSQL (ใช้ร่วมกับ `super-app-chonburi-go`)

### ติดตั้ง

```bash
# ติดตั้ง Go dependencies
go mod tidy

# ติดตั้ง Node dev tools (nodemon)
yarn install
```

### รัน

```bash
# Development — hot reload
yarn dev

# Production
yarn start

# Build binary
yarn build
```

---

## 📡 API Endpoints

> **Base URL**: `http://localhost:8081/api/v1`

| Method | Path | Auth | คำอธิบาย |
|---|---|---|---|
| `GET` | `/` | ❌ | Health check |

> Routes อื่นๆ จะเพิ่มเมื่อ implement แต่ละ feature

---

## 🔗 Related Repositories

| Repo | คำอธิบาย |
|---|---|
| [`super-app-chonburi-go`](../super-app-chonburi-go) | Admin Backend API |
| [`super-app-chonburi`](../super-app-chonburi) | Admin Frontend (Next.js) |
| [`ChonburiPlus-mb-app`](../ChonburiPlus-mb-app) | Mobile App (React Native / Expo) |

---

## 📝 Notes

- JWT ที่ออกโดย Mobile API **แยกจาก** JWT ของ Admin Backend
- ใช้ Port `8081` เพื่อให้ run parallel กับ Admin Backend ที่ Port `8080`
- ตาราง `users` ใหม่ ไม่ใช่ `admin_users` เดิม
