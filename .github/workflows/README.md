# GitHub Actions Continuous Runner

## ⚠️ Cloudflare Blocking Issue

**Problem:** GitHub Actions runners use datacenter IP addresses that Cloudflare actively blocks, even with valid cookies.

**Symptoms:**
```
INFO [channel] channel was blocked by Cloudflare (cookies configured); retrying in 10s
```

This happens repeatedly because:
1. GitHub Actions IPs are known datacenter addresses
2. Cloudflare blocks datacenter IPs regardless of cookies
3. Cookies expire quickly (30 min - 2 hours)

## 🔧 Solutions

### Option 1: Use a Residential Proxy (Recommended)

Add a residential proxy to bypass Cloudflare:

1. Get a residential proxy service (e.g., Bright Data, Smartproxy, Oxylabs)
2. Add the proxy URL to GitHub Secrets:
   - Name: `HTTPS_PROXY`
   - Value: `http://username:password@proxy.example.com:8080`
   
3. The workflow will automatically use it

### Option 2: Use a Self-Hosted Runner

Run the workflow on your own machine:

1. Go to: Settings → Actions → Runners → New self-hosted runner
2. Follow the setup instructions
3. Your home IP is less likely to be blocked

### Option 3: Reduce Polling Frequency

The workflow now uses `--interval 5` (5 minutes) instead of 1 minute to reduce request frequency and avoid triggering rate limits.

## 📋 Required Secrets

| Secret Name | Description | Required |
|------------|-------------|----------|
| `CHATURBATE_COOKIES` | `cf_clearance=...` from browser | ✅ Yes |
| `HTTPS_PROXY` | Residential proxy URL | ⚠️ Recommended |
| `GOFILE_API_KEY` | Gofile upload API key | Optional |
| `FILESTER_API_KEY` | Filester upload API key | Optional |
| `DISCORD_WEBHOOK_URL` | Discord notifications | Optional |
| `NTFY_TOKEN` | Ntfy notifications | Optional |

## 🍪 Getting Fresh Cookies

Cookies expire quickly. To get fresh ones:

1. Open Chrome in **Incognito mode**
2. Go to `https://chaturbate.com`
3. Complete Cloudflare challenge
4. Press F12 → Application → Cookies → chaturbate.com
5. Copy `cf_clearance` value
6. Update GitHub Secret immediately
7. Run workflow within 30 minutes

## 🔍 Debugging

Check the workflow logs for:

```bash
✅ Configured Chaturbate cookies from GitHub Secrets
✅ Cookie contains cf_clearance (Cloudflare bypass)
```

If you see:
```bash
❌ ERROR: CHATURBATE_COOKIES secret not configured
```

Then the secret is missing or empty.

If you see:
```bash
⚠️ WARNING: Cookie does not contain cf_clearance
```

Then your cookie format is wrong. It should be:
```
cf_clearance=YOUR_LONG_VALUE_HERE
```

## 💡 Why This Happens

Chaturbate uses Cloudflare to protect against bots and scrapers. Cloudflare:
- Blocks known datacenter IPs (like GitHub Actions)
- Requires browser-like behavior
- Expires cookies quickly
- Detects automated access patterns

**Bottom line:** GitHub Actions is not ideal for this use case. Consider running locally or using a self-hosted runner.
