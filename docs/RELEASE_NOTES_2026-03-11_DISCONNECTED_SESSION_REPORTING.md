# Release Notes - 2026-03-11 (Disconnected Session Reporting)

- Windows agent `logged_in_sessions` raporlamasi genisletildi.
- Artik yalnizca `Active` degil, `Disconnected` durumundaki oturumlar da heartbeat payload'ina eklenir.
- Veri modeli genisledi:
  - `username`
  - `session_type`: `local` | `rdp`
  - `session_state`: `active` | `disconnected`
- Server UI bu alani gosterir:
  - `AKGUN\\hakan.sen (local - disconnected)`
  - `AKGUN\\user (rdp - disconnected)`

## Dogrulama

- Lokal: `go test ./...`
- Canli binary testi: `10.6.20.172` hostunda service exe elle degistirilip heartbeat dogrulandi.
- Server DB dogrulamasi:
  - `logged_in_sessions_json` icinde `session_state` alaninin yazildigi goruldu.

## Beklenen Etki

- Kilitlenmis / baglantisi kopmus ama halen logon context'i olan kullanicilar server tarafinda gorunur.
- Uzak destek baslatmadan once makinede kullanici oturumu olup olmadigi daha dogru gorulur.
