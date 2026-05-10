# Deployment Guide

## Git Status
✅ Code committed and pushed to GitHub: https://github.com/emmanowan/family-shopping-list

## PorkBun Setup

**Important:** PorkBun is a domain registrar, not an application host. You have two options:

### Option 1: Deploy to Render (Recommended - Free)
1. Go to https://render.com and sign up
2. Click "New +" → "Web Service"
3. Connect your GitHub repo: `emmanowan/family-shopping-list`
4. Configure:
   - **Name:** family-shopping-list
   - **Runtime:** Go
   - **Build Command:** `go build -o app main.go`
   - **Start Command:** `./app`
   - **Plan:** Free
5. Add environment variable:
   - Key: `PORT`
   - Value: `10000` (Render's default port)
6. Click "Create Web Service"
7. Once deployed, copy the URL (e.g., `https://family-shopping-list.onrender.com`)

### Option 2: Connect PorkBun Domain to Render
1. In Render dashboard, go to your service → Settings → Custom Domains
2. Add your PorkBun domain (e.g., `shopping.yourdomain.com`)
3. In PorkBun DNS:
   - Add CNAME record: `shopping` → `family-shopping-list.onrender.com`
   - Or use A record pointing to Render's IP
4. Wait for DNS propagation (up to 48 hours)

### Option 3: Deploy to VPS (More Control)
If you want full control, deploy to a VPS:
1. Buy a VPS from DigitalOcean, Linode, or AWS
2. SSH into the server
3. Clone your repo: `git clone https://github.com/emmanowan/family-shopping-list.git`
4. Install Go and build the app
5. Run with systemd or use a process manager like PM2
6. Point PorkBun domain to your VPS IP address

## Database Note
The app currently uses SQLite (`shopping_list.db`). For production:
- **Render:** SQLite file will persist in the disk
- **VPS:** File persists on the server
- **Better option:** Switch to PostgreSQL for better scalability

## Quick Render Deploy
The app is ready for Render deployment with these features:
- SQLite database (auto-created)
- Dinner poll with voting
- Spin the wheel game
- Modern Tailwind CSS UI
- Name autocomplete from previous entries
