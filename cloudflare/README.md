# Cloudflare Worker — `get.ottomind.ai/vh`

Tek bir Worker, `get.ottomind.ai/vh` ve `get.ottomind.ai/vh/<version>` isteklerini
GitHub'daki `install.sh` dosyasına proxy'ler. `Content-Type: text/plain`
döndürür, böylece `curl | bash` doğal olarak çalışır.

## Deploy

```bash
# bir kerelik
npm i -g wrangler

cd cloudflare
wrangler login                # tarayıcıda Cloudflare oturumu
wrangler deploy
```

Wrangler `wrangler.toml`'daki `routes`'u kullanarak ottomind.ai zonuna
otomatik bağlar — manuel route eklemen gerekmez.

## Test

```bash
curl -fsSL https://get.ottomind.ai/vh | head -20
# #!/usr/bin/env bash ... ile başlamalı

curl -fsSL https://get.ottomind.ai/vh/0.1.0 | head -3
# export VPS_HEALTH_VERSION="0.1.0" satırı eklenmiş olmalı
```

## Alternatif: Cloudflare Redirect Rule (Worker'sız)

Worker kullanmak istemezsen, Cloudflare dashboard → Rules → Redirect Rules:

```
When incoming requests match... URI Path equals /vh
Then... Static redirect to https://raw.githubusercontent.com/19011022/vps-health/main/install.sh
Status: 302
Preserve query string: yes
```

`curl -fsSL` `-L` (follow redirect) içerdiği için bu da çalışır. Ama:
- Sürüm pinleme yapamazsın (`/vh/0.1.0` çalışmaz)
- `Content-Type` GitHub'ın döndürdüğü olur (genelde `text/plain` olur ama garanti değil)

Worker daha temiz. Aylık 100k istek free tier'da.
