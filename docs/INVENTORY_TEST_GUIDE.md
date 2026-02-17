# Inventory Test Guide

Bu rehber, agent inventory tarama/sync ve degisim gecmisi akislarini adim adim dogrulamak icin kullanilir.

## Test Ortami

- Windows host: `10.6.20.172`
- SSH kullanici: `apptest`
- SSH sifre: `1234asd!!!`

## 1. On Kosul

- `AppCenterAgent` service calisiyor olmali.
- Agent server'a heartbeat atabiliyor olmali.
- Log yolu:
  - `C:\ProgramData\AppCenter\logs\agent.log`

Hizli kontrol:

```powershell
Get-Service AppCenterAgent
& 'C:\Program Files\AppCenter\appcenter-tray-cli.exe' get_status
```

## 2. Ilk Inventory Sync Dogrulamasi

1. Service'i restart et:

```powershell
Restart-Service AppCenterAgent
```

2. Log'da tarama ve submit satirlarini kontrol et:

```powershell
Get-Content 'C:\ProgramData\AppCenter\logs\agent.log' -Tail 120
```

Beklenen ornek satirlar:

- `inventory force scan: ... items, hash=...`
- `heartbeat ok: status=ok`
- `inventory submitted: Inventory updated (...)`

## 3. Server Tarafinda Inventory Kaydi Dogrulama

1. Token al:

```bash
curl -X POST http://10.6.100.170:8000/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

2. Agent inventory listesini cek:

```bash
curl -H "Authorization: Bearer <TOKEN>" \
  "http://10.6.100.170:8000/api/v1/agents/54d2ad5c-5b66-477d-82da-e5a22ef6dc01/inventory"
```

Beklenen:

- `total > 0`
- `items` listesi dolu

## 4. Degisim Gecmisi Testi

1. Baslangic snapshot olustur (service restart veya scan interval bekleme).
2. Windows'ta bir yazilimi kaldir/kur (ornek: 7-Zip uninstall/install).
3. Sonraki inventory submit'i bekle (log'da `inventory submitted`).
4. Change history endpoint'ini cek:

```bash
curl -H "Authorization: Bearer <TOKEN>" \
  "http://10.6.100.170:8000/api/v1/agents/54d2ad5c-5b66-477d-82da-e5a22ef6dc01/inventory/changes?limit=50&offset=0"
```

Beklenen:

- `total > 0`
- `change_type` alanlari: `installed` / `removed` / `updated`

## 5. UI Uzerinden Dogrulama

- `Agents -> <agent> -> Yuklu Yazilimlar`:
  - inventory tablo verisi gorunmeli
  - degisim gecmisi listesi dolmali

