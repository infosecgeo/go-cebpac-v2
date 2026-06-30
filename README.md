# CebuPacific Payment Processor - License-Based SaaS Platform

A comprehensive license-based SaaS platform for CebuPacific payment processing with integrated Telegram bot management, admin dashboard, and credit system.

## 🚀 Features

### User Features
- **License-Based Authentication**: Users log in with license keys (no username/password)
- **Telegram Bot Integration**: Link license to Telegram account for credit management
- **Credit System**: Pay-per-use model with real-time credit tracking
- **Dual-Login Prevention**: Automatic session termination when logging in from another device
- **Real-Time Progress Tracking**: Detailed progress logs with timestamps and status updates
- **Zero Credit UI Restrictions**: Automatic form disabling when credits reach zero

### Admin Features
- **Complete Admin Dashboard**: Manage all aspects of the platform
- **User Management**: View, edit, delete users, modify credits
- **License Management**: Generate, assign, revoke licenses
- **Top-up Management**: View and manage credit top-up requests
- **System Settings**: Configure API keys, proxy URLs, Telegram bot settings
- **QR Code Management**: Upload payment QR codes for user top-ups

### Telegram Bot Features
- **/start** - Welcome message and bot information
- **/link <license_key>** - Link license to Telegram account (1 license per account)
- **/credits** - Check current credit balance
- **/topup <amount>** - Request credit top-up with payment QR code
- **/status** - View account status and details
- **Transaction Notifications**: Real-time notifications to configured channel
- **Admin Approval System**: Approve/deny top-ups via inline buttons in Telegram

## 📋 Prerequisites

- Go 1.21 or higher
- Node.js 16+ (for frontend development, optional)
- Telegram Bot Token (create via [@BotFather](https://t.me/BotFather))
- Two Telegram channels:
  - Notification channel (for transaction notifications)
  - Approval channel (for admin top-up approvals)

## 🔧 Installation

### 1. Clone the Repository

```bash
git clone https://github.com/infosecgeo/go-cebpac-v2.git
cd go-cebpac-v2
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Create Storage Directories

```bash
mkdir -p storage/qr_codes
```

### 4. Configure Environment Variables

Copy the example environment file and configure:

```bash
cp .env.example .env
```

Edit `.env` with your configuration:

```env
# Server Configuration
PORT=8080
JWT_SECRET=your-secret-key-change-me-in-production

# Telegram Bot Configuration
TELEGRAM_BOT_TOKEN=your-bot-token-from-botfather
TELEGRAM_NOTIFICATION_CHANNEL_ID=-1001234567890
TELEGRAM_ADMIN_APPROVAL_CHANNEL_ID=-1001234567891

# Admin Credentials (First-time setup)
ADMIN_USERNAME=admin
ADMIN_PASSWORD=changeme123

# Optional: Can also be configured via admin panel
DEFAULT_PROXY_URL=username:password@host:port
CEBUPAC_API_KEY=your-api-key
```

### 5. Initialize Database

The database will be automatically created on first run at `./storage/database.json`

## 🏃 Running the Application

### Development Mode

```bash
go run backend/main.go
```

### Production Build

```bash
go build -o cebupac-processor backend/main.go
./cebupac-processor
```

The application will be available at:
- User Interface: `http://localhost:8080/`
- Admin Dashboard: `http://localhost:8080/admin.html`

## 📱 Telegram Bot Setup

### 1. Create a Bot

1. Message [@BotFather](https://t.me/BotFather) on Telegram
2. Send `/newbot` and follow the instructions
3. Copy the bot token provided

### 2. Create Channels

1. Create two private channels in Telegram
2. Add your bot as an administrator to both channels
3. Get channel IDs:
   - Forward a message from each channel to [@userinfobot](https://t.me/userinfobot)
   - Copy the channel ID (starts with `-100`)

### 3. Configure Bot

Set the bot token and channel IDs in `.env` or via the admin dashboard settings panel.

### 4. Test the Bot

1. Start your application
2. Message your bot on Telegram
3. Send `/start` to verify it's working

## 👨‍💼 Admin Dashboard Access

### First-Time Login

1. Navigate to `http://localhost:8080/admin.html`
2. Login with default credentials:
   - Username: `admin`
   - Password: `changeme123`
3. **IMPORTANT**: Change the default password immediately in production!

### Admin Dashboard Sections

- **Overview**: View stats (total users, active licenses, pending top-ups)
- **Users**: Manage users, modify credits, change status
- **Licenses**: Generate new licenses, revoke existing ones
- **Top-ups**: View and filter top-up requests
- **Settings**: Configure API keys, Telegram bot, QR codes

## 👤 User Registration & Usage Flow

### Registration

1. User navigates to `http://localhost:8080/`
2. Clicks "Register here"
3. Enters license key (provided by admin)
4. Accepts Terms and Conditions
5. Receives Telegram bot link instructions
6. Links Telegram account via `/link <license_key>` command
7. Requests first top-up via `/topup` command
8. Uploads payment receipt
9. Waits for admin approval
10. Account becomes active after approval

### Using the System

1. Login with license key
2. Ensure credits > 0
3. Fill payment form
4. Submit transaction
5. View progress logs in real-time
6. Credits deducted on successful transaction

## 🔐 Security Features

### License-Based Authentication
- No passwords for regular users
- License keys generated with cryptographically secure random values
- Format: `XXXXX-XXXXX-XXXXX-XXXXX`

### Session Management
- JWT-based authentication
- Automatic session expiration
- Dual-login prevention with immediate termination

### Admin Protection
- Separate admin authentication endpoint
- Admin-only middleware for sensitive routes
- Audit logging for admin actions (future enhancement)

## 🛠️ API Endpoints

### Public Endpoints
- `POST /api/v1/auth/login` - User login (license key)
- `POST /api/v1/auth/register` - User registration
- `POST /api/v1/auth/admin/login` - Admin login

### Admin Endpoints (Requires Admin Token)
- `GET /api/v1/admin/settings` - Get system settings
- `PUT /api/v1/admin/settings` - Update settings
- `POST /api/v1/admin/settings/qr` - Upload QR code
- `GET /api/v1/admin/users` - List users
- `PUT /api/v1/admin/users/:id` - Update user
- `DELETE /api/v1/admin/users/:id` - Delete user
- `GET /api/v1/admin/licenses` - List licenses
- `POST /api/v1/admin/licenses` - Create license
- `DELETE /api/v1/admin/licenses/:id` - Revoke license
- `GET /api/v1/admin/topups` - List top-up requests

### WebSocket Events
- `progress` - Transaction progress updates
- `stats_update` - System stats updates
- `proxy_change` - Proxy change notifications
- `task_error` - Task errors
- `kicked_out` - Session termination (dual-login)
- `credit_update` - Credit balance updates
- `progress_log` - Detailed progress logs

## 📊 Database Schema

### Collections
- `users` - User accounts with Telegram integration
- `licenses` - License keys with status and expiry
- `sessions` - Active user sessions
- `topup_requests` - Credit top-up requests
- `admin_settings` - System configuration
- `transactions` - Transaction history

## 🚨 Troubleshooting

### Telegram Bot Not Responding
1. Check bot token is correct
2. Verify bot is running (check logs)
3. Ensure channels are configured correctly
4. Check bot has admin access in channels

### Users Can't Login
1. Verify license is active (check admin dashboard)
2. Ensure license is not already linked to another Telegram account
3. Check JWT_SECRET is set
4. Verify user has accepted terms and completed first top-up

### Credits Not Deducting
1. Check credit update WebSocket handler
2. Verify transaction success logic
3. Check user balance in admin dashboard

### Admin Can't Access Dashboard
1. Verify admin credentials
2. Check `/api/v1/auth/admin/login` endpoint
3. Ensure admin-only middleware is working

## 📝 Development

### Project Structure
```
├── backend/
│   ├── database/        # Database models and helpers
│   ├── routes/          # HTTP route handlers
│   ├── telegram/        # Telegram bot implementation
│   ├── websocket/       # WebSocket hub and handlers
│   └── main.go          # Application entry point
├── frontend/
│   └── assets/
│       ├── css/         # Stylesheets
│       └── js/          # Frontend JavaScript (ES6 modules)
│           ├── components/       # UI components
│           │   └── admin/        # Admin dashboard components
│           ├── modules/          # Core modules
│           └── services/         # API services
├── public/
│   ├── index.html       # User interface
│   └── admin.html       # Admin dashboard
└── storage/             # Data storage
    ├── database.json    # JSON database
    └── qr_codes/        # Payment QR codes
```

### Adding New Features
1. Backend: Add handlers in `backend/routes/`
2. Frontend: Create components in `frontend/assets/js/components/`
3. Telegram: Extend bot commands in `backend/telegram/handlers.go`

## 🐛 Known Issues & Future Enhancements

### Phase 12: Integration & Testing (Pending)
- [ ] End-to-end testing of registration flow
- [ ] Telegram bot integration testing
- [ ] Dual-login prevention testing
- [ ] Credit deduction verification
- [ ] Admin dashboard testing

### Phase 13: Security & Validation (Pending)
- [ ] Rate limiting on top-up requests
- [ ] Payment receipt validation (file type, size)
- [ ] Input sanitization for Telegram bot
- [ ] 2FA for admin accounts
- [ ] Audit logging for admin actions

### Future Enhancements
- Transaction history viewer
- Email notifications
- API rate limiting
- Webhook support for Telegram bot
- Multi-admin support
- User activity logs
- Batch license generation

## 📜 License

This project is proprietary software. All rights reserved.

## 🤝 Support

For issues and questions:
- Create an issue in this repository
- Contact the administrator

## 🔄 Version History

### v2.0.0 (Current)
- Complete SaaS transformation with license-based authentication
- Telegram bot integration for credit management
- Full admin dashboard
- Dual-login prevention
- Real-time progress tracking
- Zero credit UI restrictions

### v1.0.0
- Initial payment processor implementation
