//go:build windows

package tray

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type ipcStoreResponse struct {
	Status  string       `json:"status"`
	Message string       `json:"message"`
	Data    StorePayload `json:"data"`
}

func OpenStoreNativeUIStandalone() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exeDir := filepath.Dir(exePath)
	cliPath := filepath.Join(exeDir, "appcenter-tray-cli.exe")
	if _, err := os.Stat(cliPath); err != nil {
		// Fallback: same binary can still serve IPC actions.
		cliPath = exePath
	}

	out, err := exec.Command(cliPath, "get_store").CombinedOutput()
	if err != nil {
		return fmt.Errorf("get_store failed: %v (%s)", err, string(out))
	}

	var resp ipcStoreResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return fmt.Errorf("invalid store response: %w", err)
	}
	if resp.Status != "ok" {
		msg := resp.Message
		if msg == "" {
			msg = "store unavailable"
		}
		return fmt.Errorf(msg)
	}

	appsJSON, err := json.Marshal(resp.Data.Apps)
	if err != nil {
		return err
	}
	appsB64 := base64.StdEncoding.EncodeToString(appsJSON)
	cliB64 := base64.StdEncoding.EncodeToString([]byte(cliPath))

	var ps bytes.Buffer
	ps.WriteString("$ErrorActionPreference='Stop'\n")
	ps.WriteString("if (-not [Environment]::UserInteractive) { throw 'Store UI requires interactive user session (RDP/Console).' }\n")
	ps.WriteString("Add-Type -AssemblyName System.Windows.Forms\n")
	ps.WriteString("Add-Type -AssemblyName System.Drawing\n")
	ps.WriteString("$apps = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('" + appsB64 + "')) | ConvertFrom-Json\n")
	ps.WriteString("$cli  = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('" + cliB64 + "'))\n")
	ps.WriteString("$form = New-Object System.Windows.Forms.Form\n")
	ps.WriteString("$form.Text = 'AppCenter Store'\n")
	ps.WriteString("$form.StartPosition = 'CenterScreen'\n")
	ps.WriteString("$form.Size = New-Object System.Drawing.Size(780,560)\n")
	ps.WriteString("$form.TopMost = $true\n")
	ps.WriteString("$txt = New-Object System.Windows.Forms.TextBox\n")
	ps.WriteString("$txt.Location = New-Object System.Drawing.Point(10,10)\n")
	ps.WriteString("$txt.Size = New-Object System.Drawing.Size(620,24)\n")
	ps.WriteString("$btnInstall = New-Object System.Windows.Forms.Button\n")
	ps.WriteString("$btnInstall.Text = 'Kur'\n")
	ps.WriteString("$btnInstall.Location = New-Object System.Drawing.Point(640,9)\n")
	ps.WriteString("$btnInstall.Size = New-Object System.Drawing.Size(120,26)\n")
	ps.WriteString("$grid = New-Object System.Windows.Forms.DataGridView\n")
	ps.WriteString("$grid.Location = New-Object System.Drawing.Point(10,42)\n")
	ps.WriteString("$grid.Size = New-Object System.Drawing.Size(750,470)\n")
	ps.WriteString("$grid.ReadOnly = $true\n")
	ps.WriteString("$grid.SelectionMode = 'FullRowSelect'\n")
	ps.WriteString("$grid.MultiSelect = $false\n")
	ps.WriteString("$grid.AutoGenerateColumns = $false\n")
	ps.WriteString("$grid.AllowUserToAddRows = $false\n")
	ps.WriteString("$grid.AllowUserToDeleteRows = $false\n")
	ps.WriteString("$grid.RowHeadersVisible = $false\n")
	ps.WriteString("$binding = New-Object System.Windows.Forms.BindingSource\n")
	ps.WriteString("$allRows = New-Object System.Collections.ArrayList\n")
	ps.WriteString("foreach ($a in $apps) {\n")
	ps.WriteString("  [void]$allRows.Add([PSCustomObject]@{id=$a.id; name=$a.display_name; version=$a.version; installed=([bool]$a.installed); category=$a.category; desc=$a.description; size_mb=$a.file_size_mb})\n")
	ps.WriteString("}\n")
	ps.WriteString("$cols = @(\n")
	ps.WriteString("  @{Name='ID'; DataPropertyName='id'; Width=60},\n")
	ps.WriteString("  @{Name='Uygulama'; DataPropertyName='name'; Width=220},\n")
	ps.WriteString("  @{Name='Versiyon'; DataPropertyName='version'; Width=90},\n")
	ps.WriteString("  @{Name='Yüklü'; DataPropertyName='installed'; Width=70},\n")
	ps.WriteString("  @{Name='Kategori'; DataPropertyName='category'; Width=120},\n")
	ps.WriteString("  @{Name='Boyut(MB)'; DataPropertyName='size_mb'; Width=85},\n")
	ps.WriteString("  @{Name='Açıklama'; DataPropertyName='desc'; Width=200}\n")
	ps.WriteString(")\n")
	ps.WriteString("foreach ($c in $cols) { $col = New-Object System.Windows.Forms.DataGridViewTextBoxColumn; $col.Name=$c.Name; $col.DataPropertyName=$c.DataPropertyName; $col.Width=$c.Width; [void]$grid.Columns.Add($col) }\n")
	ps.WriteString("function Apply-Filter {\n")
	ps.WriteString("  $q = ($txt.Text | ForEach-Object { $_.ToLowerInvariant() })\n")
	ps.WriteString("  $rows = New-Object System.Collections.ArrayList\n")
	ps.WriteString("  foreach($r in $allRows){\n")
	ps.WriteString("    if([string]::IsNullOrWhiteSpace($q) -or ($r.name -as [string]).ToLowerInvariant().Contains($q) -or ($r.desc -as [string]).ToLowerInvariant().Contains($q) -or ($r.category -as [string]).ToLowerInvariant().Contains($q)){ [void]$rows.Add($r) }\n")
	ps.WriteString("  }\n")
	ps.WriteString("  $binding.DataSource = $rows\n")
	ps.WriteString("  $grid.DataSource = $binding\n")
	ps.WriteString("}\n")
	ps.WriteString("$txt.Add_TextChanged({ Apply-Filter })\n")
	ps.WriteString("$btnInstall.Add_Click({\n")
	ps.WriteString("  if ($grid.SelectedRows.Count -eq 0) { [System.Windows.Forms.MessageBox]::Show('Önce bir uygulama seçin.','AppCenter Store',[System.Windows.Forms.MessageBoxButtons]::OK,[System.Windows.Forms.MessageBoxIcon]::Information) | Out-Null; return }\n")
	ps.WriteString("  $row = $grid.SelectedRows[0].DataBoundItem\n")
	ps.WriteString("  if ($null -eq $row) { return }\n")
	ps.WriteString("  $id = [int]$row.id\n")
	ps.WriteString("  $name = [string]$row.name\n")
	ps.WriteString("  $out = & $cli install_from_store $id 2>&1 | Out-String\n")
	ps.WriteString("  [System.Windows.Forms.MessageBox]::Show(($name + \"\\n\\n\" + $out),'Kurulum İsteği',[System.Windows.Forms.MessageBoxButtons]::OK,[System.Windows.Forms.MessageBoxIcon]::Information) | Out-Null\n")
	ps.WriteString("})\n")
	ps.WriteString("$form.Controls.Add($txt)\n")
	ps.WriteString("$form.Controls.Add($btnInstall)\n")
	ps.WriteString("$form.Controls.Add($grid)\n")
	ps.WriteString("$form.Add_Shown({ $form.Activate() })\n")
	ps.WriteString("Apply-Filter\n")
	ps.WriteString("[void]$form.ShowDialog()\n")

	psPath := filepath.Join(os.TempDir(), "appcenter-store-native.ps1")
	if err := os.WriteFile(psPath, ps.Bytes(), 0o600); err != nil {
		return err
	}

	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-File", psPath)
	out, cmdErr := cmd.CombinedOutput()
	if cmdErr != nil {
		trayDiagf("native store ui failed: %v (%s)", cmdErr, string(out))
		return fmt.Errorf("native store ui failed: %v (%s)", cmdErr, string(out))
	}
	return nil
}
