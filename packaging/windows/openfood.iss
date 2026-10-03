#ifndef AppVersion
 #define AppVersion "0.2.0-alpha.1"
#endif
[Setup]
AppId={{A989B1D0-0DAF-4878-8155-21FE89762AB6}
AppName=OpenFood
AppVersion={#AppVersion}
AppPublisher=OpenFood contributors
DefaultDirName={localappdata}\Programs\OpenFood
DefaultGroupName=OpenFood
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\..\dist
OutputBaseFilename=OpenFood-{#AppVersion}-windows-x64-setup
Compression=lzma2
SolidCompression=yes
CloseApplications=yes
RestartApplications=no
SetupMutex=Local\OpenFoodInstaller
UninstallDisplayIcon={app}\OpenFood.exe
LicenseFile=..\..\LICENSE
WizardStyle=modern
[Files]
Source: "..\..\dist\windows\OpenFood.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\dist\windows\postgresql\*"; DestDir: "{app}\postgresql"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "..\..\README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\dist\windows\licenses\*"; DestDir: "{app}\licenses"; Flags: ignoreversion
[Languages]
Name: "brazilianportuguese"; MessagesFile: "compiler:Languages\BrazilianPortuguese.isl"
[Icons]
Name: "{group}\OpenFood"; Filename: "{app}\OpenFood.exe"
Name: "{group}\Desinstalar OpenFood"; Filename: "{uninstallexe}"
[Run]
Filename: "{app}\OpenFood.exe"; Description: "Abrir OpenFood"; Flags: nowait postinstall skipifsilent
[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueName: "OpenFood"; Flags: uninsdeletevalue
[Code]
function PrepareToInstall(var NeedsRestart: Boolean): String;
var ResultCode: Integer;
begin
  Result := '';
  if FileExists(ExpandConstant('{app}\OpenFood.exe')) then begin
    if not Exec(ExpandConstant('{app}\OpenFood.exe'), '--prepare-update', '', SW_HIDE, ewWaitUntilTerminated, ResultCode) then
      Result := 'Não foi possível preparar o backup. Encerre OpenFood pela bandeja e tente novamente.'
    else if ResultCode <> 0 then
      Result := 'Backup anterior à atualização falhou. Atualização interrompida.';
  end;
end;
// No data directory is listed under UninstallDelete: data is preserved by default.
