# Windows MSI Test Guide

Bu rehber, agent installer katmanindaki MSI akislarini Windows uzerinde dogrulamak icin adim adim test plani sunar.

## 1. Amac

Dogrulanan konular:

- `internal/installer/msi_windows.go` uzerinden `msiexec` cagrisinin calismasi
- Basarili kurulumda `exit_code=0`
- Reboot-required exit code'larin agent tarafinda basari sayilmasi:
  - `3010` (reboot required)
  - `1641` (reboot initiated)
- Hatali kurulumda dogru hata/exit code donusu
- Silent argumanlarin uygulandiginin dogrulanmasi

## 2. Gereksinimler

- Windows 10/11 veya Server 2016+
- Administrator yetkili PowerShell/CMD
- Test MSI dosyasi (ornek: `7zip-x64.msi`)
- Agent binary (CI artifact veya lokal build):
  - `appcenter-service.exe`

## 3. Ortam Hazirligi

1. Klasorleri olustur:

```powershell
New-Item -ItemType Directory -Force "C:\Program Files\AppCenter" | Out-Null
New-Item -ItemType Directory -Force "C:\ProgramData\AppCenter\downloads" | Out-Null
New-Item -ItemType Directory -Force "C:\ProgramData\AppCenter\logs" | Out-Null
```

2. Binary kopyala:

```powershell
Copy-Item .\appcenter-service.exe "C:\Program Files\AppCenter\appcenter-service.exe" -Force
```

3. MSI dosyasini indirilenler klasorune koy:

```powershell
Copy-Item .\7zip-x64.msi "C:\ProgramData\AppCenter\downloads\7zip-x64.msi" -Force
```

## 4. Test Senaryolari

### Senaryo A: Basarili MSI kurulum (silent)

1. Komut:

```powershell
msiexec /i "C:\ProgramData\AppCenter\downloads\7zip-x64.msi" /qn /norestart
$LASTEXITCODE
```

2. Beklenen:

- `0` donmeli
- Uygulama Programs and Features listesinde gorunmeli

3. Kayit:

- Kurulum suresi
- Exit code

### Senaryo B: Hatali MSI yolu

1. Komut:

```powershell
msiexec /i "C:\ProgramData\AppCenter\downloads\NOT_FOUND.msi" /qn /norestart
$LASTEXITCODE
```

2. Beklenen:

- `0` disinda bir kod
- Agent tarafinda hata metni yakalanabilir olmali

### Senaryo C: Gecersiz arguman

1. Komut:

```powershell
msiexec /i "C:\ProgramData\AppCenter\downloads\7zip-x64.msi" /badarg
$LASTEXITCODE
```

2. Beklenen:

- `0` disi kod
- Hata metni donmeli

Not (SSH/non-interactive):

- Bu senaryo bazen MSI UI beklemesine takilabilir.
- Otomasyon icin alternatif olarak gecersiz MSI paketi testi kullanin:

```powershell
Set-Content -Path "C:\ProgramData\AppCenter\downloads\invalid.msi" -Value "not-a-real-msi"
msiexec /i "C:\ProgramData\AppCenter\downloads\invalid.msi" /qn /norestart
$LASTEXITCODE
```

- Beklenen: `1620` (invalid package)

## 5. Agent Koduyla Entegre Dogrulama

Bu adim installer fonksiyonunun gercek calismasini dolayli dogrular.

1. Server tarafinda bir deployment olustur (MSI app)
2. Agent'i calistir, heartbeat ile task cekmesini sagla
3. Kurulum tamamlandiginda task status endpoint kontrol et:

- Basarili ise `status=success`, `exit_code=0`
- Hatada `status=failed`, `exit_code!=0`

## 6. Log Toplama

- Windows Event Viewer (Application)
- Agent log dosyasi:
  - `C:\ProgramData\AppCenter\logs\agent.log`
- MSI verbose log (opsiyonel):

```powershell
msiexec /i "C:\ProgramData\AppCenter\downloads\7zip-x64.msi" /qn /L*v "C:\ProgramData\AppCenter\logs\msi-install.log"
```

## 7. Test Rapor Formati

Her senaryo icin su satiri doldur:

- Senaryo:
- Komut:
- Beklenen:
- Gerceklesen:
- Exit code:
- Not:
