# Critter Bid frontend

Next.js 16（App Router）。說明見專案根目錄 [README.md](../README.md) 與 [AGENTS.md](../AGENTS.md)（ADR-13 列出此版本 Next.js 的注意事項）。

```bash
cp .env.example .env.local
npm install
npm run dev      # http://localhost:3000（/api 代理至 BACKEND_URL）
npm test && npm run typecheck && npm run lint && npm run build
```
