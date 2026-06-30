# Deployment Guide

This guide covers deploying the CebuPacific Payment Processor to production.

## 🎯 Pre-Deployment Checklist

### Security
- [ ] Change default admin credentials
- [ ] Generate strong JWT secret (64+ characters)
- [ ] Set secure bot token
- [ ] Configure HTTPS/TLS certificates
- [ ] Review CSP headers
- [ ] Enable firewall rules
- [ ] Set up backup strategy

### Configuration
- [ ] Configure production environment variables
- [ ] Set up Telegram bot and channels
- [ ] Test bot connectivity
- [ ] Upload payment QR code via admin panel
- [ ] Generate initial licenses
- [ ] Test license authentication flow

### Infrastructure
- [ ] Provision server (2GB RAM minimum)
- [ ] Install Go 1.21+
- [ ] Set up reverse proxy (Nginx/Caddy)
- [ ] Configure domain and DNS
- [ ] Set up SSL/TLS
- [ ] Configure systemd service

## 🏗️ Server Requirements

### Minimum Specifications
- **CPU**: 1 core
- **RAM**: 2GB
- **Storage**: 10GB
- **OS**: Ubuntu 22.04 LTS / Debian 12 / CentOS 8+
- **Network**: Public IP with ports 80/443 open

### Recommended Specifications
- **CPU**: 2 cores
- **RAM**: 4GB
- **Storage**: 20GB SSD
- **OS**: Ubuntu 22.04 LTS

## 📦 Installation Steps

### 1. System Preparation

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Go
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify Go installation
go version
```

### 2. Application Setup

```bash
# Create application user
sudo useradd -r -s /bin/false cebupac

# Create application directory
sudo mkdir -p /opt/cebupac
sudo chown cebupac:cebupac /opt/cebupac

# Switch to application user
sudo -u cebupac -s

# Clone repository
cd /opt/cebupac
git clone https://github.com/infosecgeo/go-cebpac-v2.git .

# Install dependencies
go mod download

# Build application
go build -o cebupac-processor backend/main.go

# Create storage directories
mkdir -p storage/qr_codes
```

### 3. Environment Configuration

```bash
# Create production environment file
sudo -u cebupac nano /opt/cebupac/.env
```

Add the following configuration:

```env
# Server
PORT=8080
HOST=127.0.0.1

# Security
JWT_SECRET=generate-a-very-long-random-string-here-64-chars-minimum

# Telegram
TELEGRAM_BOT_TOKEN=your-production-bot-token
TELEGRAM_NOTIFICATION_CHANNEL_ID=-100your-notification-channel-id
TELEGRAM_ADMIN_APPROVAL_CHANNEL_ID=-100your-approval-channel-id

# Admin (Change these immediately after first login!)
ADMIN_USERNAME=admin
ADMIN_PASSWORD=strong-random-password-here

# Database
DB_PATH=./storage/database.json

# Storage
STORAGE_PATH=./storage
QR_CODE_PATH=./storage/qr_codes
```

### 4. Systemd Service Setup

Create systemd service file:

```bash
sudo nano /etc/systemd/system/cebupac.service
```

Add the following:

```ini
[Unit]
Description=CebuPacific Payment Processor
After=network.target

[Service]
Type=simple
User=cebupac
Group=cebupac
WorkingDirectory=/opt/cebupac
ExecStart=/opt/cebupac/cebupac-processor
Restart=always
RestartSec=10

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/cebupac/storage

# Environment
EnvironmentFile=/opt/cebupac/.env

[Install]
WantedBy=multi-user.target
```

Enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable cebupac
sudo systemctl start cebupac
sudo systemctl status cebupac
```

### 5. Reverse Proxy Setup (Nginx)

Install Nginx:

```bash
sudo apt install nginx -y
```

Create Nginx configuration:

```bash
sudo nano /etc/nginx/sites-available/cebupac
```

Add the following:

```nginx
server {
    listen 80;
    server_name your-domain.com;

    # Redirect to HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;

    # SSL Configuration (use certbot for Let's Encrypt)
    ssl_certificate /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    # Security Headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # Logging
    access_log /var/log/nginx/cebupac_access.log;
    error_log /var/log/nginx/cebupac_error.log;

    # Application
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }

    # WebSocket Support
    location /ws {
        proxy_pass http://127.0.0.1:8080/ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 86400;
    }

    # Static files caching
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg)$ {
        proxy_pass http://127.0.0.1:8080;
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
}
```

Enable the site:

```bash
sudo ln -s /etc/nginx/sites-available/cebupac /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl restart nginx
```

### 6. SSL Certificate Setup (Let's Encrypt)

```bash
# Install Certbot
sudo apt install certbot python3-certbot-nginx -y

# Obtain certificate
sudo certbot --nginx -d your-domain.com

# Auto-renewal
sudo certbot renew --dry-run
```

## 🔐 Post-Deployment Security

### 1. Change Default Credentials

Access admin dashboard at `https://your-domain.com/admin.html` and:
1. Login with default credentials
2. Navigate to Settings
3. Change admin password (store in secure location)

### 2. Configure Firewall

```bash
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
sudo ufw status
```

### 3. Set Up Backups

Create backup script:

```bash
sudo nano /usr/local/bin/backup-cebupac.sh
```

Add:

```bash
#!/bin/bash
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/backup/cebupac"
mkdir -p $BACKUP_DIR

# Backup database
cp /opt/cebupac/storage/database.json $BACKUP_DIR/database_$DATE.json

# Backup QR codes
tar -czf $BACKUP_DIR/qr_codes_$DATE.tar.gz /opt/cebupac/storage/qr_codes

# Keep only last 30 days
find $BACKUP_DIR -type f -mtime +30 -delete

echo "Backup completed: $DATE"
```

Make executable and schedule:

```bash
sudo chmod +x /usr/local/bin/backup-cebupac.sh
sudo crontab -e
# Add: 0 2 * * * /usr/local/bin/backup-cebupac.sh
```

## 📊 Monitoring

### Application Logs

```bash
# View application logs
sudo journalctl -u cebupac -f

# View Nginx access logs
sudo tail -f /var/log/nginx/cebupac_access.log

# View Nginx error logs
sudo tail -f /var/log/nginx/cebupac_error.log
```

### Health Check

Create a simple health check script:

```bash
#!/bin/bash
STATUS=$(systemctl is-active cebupac)
if [ "$STATUS" != "active" ]; then
    echo "Service is down! Status: $STATUS"
    # Send alert (email, Telegram, etc.)
fi
```

## 🔄 Updates and Maintenance

### Updating the Application

```bash
# Stop service
sudo systemctl stop cebupac

# Backup current version
sudo -u cebupac cp /opt/cebupac/cebupac-processor /opt/cebupac/cebupac-processor.backup

# Pull updates
cd /opt/cebupac
sudo -u cebupac git pull

# Rebuild
sudo -u cebupac go build -o cebupac-processor backend/main.go

# Start service
sudo systemctl start cebupac
sudo systemctl status cebupac
```

### Rollback Procedure

```bash
# Stop service
sudo systemctl stop cebupac

# Restore backup
sudo -u cebupac mv /opt/cebupac/cebupac-processor.backup /opt/cebupac/cebupac-processor

# Start service
sudo systemctl start cebupac
```

## 🚨 Troubleshooting

### Service Won't Start

```bash
# Check logs
sudo journalctl -u cebupac -n 50 --no-pager

# Check permissions
ls -la /opt/cebupac/storage

# Verify environment
sudo systemctl cat cebupac
```

### WebSocket Connection Issues

1. Check Nginx configuration
2. Verify proxy settings
3. Check firewall rules
4. Review browser console for errors

### Telegram Bot Not Working

1. Verify bot token in `.env`
2. Check channel IDs are correct
3. Ensure bot is admin in both channels
4. Review application logs for errors

### Database Issues

```bash
# Check database file
cat /opt/cebupac/storage/database.json | jq .

# Restore from backup
sudo -u cebupac cp /backup/cebupac/database_YYYYMMDD_HHMMSS.json /opt/cebupac/storage/database.json
sudo systemctl restart cebupac
```

## 📞 Support

For production issues:
- Check logs first
- Review this documentation
- Contact system administrator
- Create issue in repository

## ✅ Final Checklist

Before going live:
- [ ] All services running and healthy
- [ ] SSL certificate valid
- [ ] Admin credentials changed
- [ ] Telegram bot responding
- [ ] Backups configured and tested
- [ ] Monitoring in place
- [ ] Firewall configured
- [ ] DNS configured correctly
- [ ] Test user registration flow
- [ ] Test payment processing
- [ ] Test admin dashboard access
- [ ] Document any custom configuration
