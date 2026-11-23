[Setup]
AppId=Auto-Test.Desktop
AppName=Auto-Test
AppVersion=0.1.0
DefaultDirName={localappdata}\Programs\Auto-Test
DefaultGroupName=Auto-Test
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputBaseFilename=Auto-Test-windows-x64-setup
Compression=lzma2/fast
SolidCompression=yes
SetupIconFile={#Payload}\app-icon.ico
UninstallDisplayIcon={app}\app-icon.ico

[Files]
Source: "{#Payload}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\Auto-Test"; Filename: "{app}\python\pythonw.exe"; Parameters: """{app}\desktop.py"""; WorkingDir: "{app}"; IconFilename: "{app}\app-icon.ico"; AppUserModelID: "AutoTest.Desktop"
Name: "{group}\Stop Auto-Test"; Filename: "{app}\python\pythonw.exe"; Parameters: """{app}\desktop.py"" --stop --notify"; WorkingDir: "{app}"; IconFilename: "{app}\app-icon.ico"

[Run]
Filename: "{app}\python\pythonw.exe"; Parameters: """{app}\desktop.py"""; Description: "Open Auto-Test"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{app}\python\pythonw.exe"; Parameters: """{app}\desktop.py"" --stop"; RunOnceId: "StopDesktop"; Flags: runhidden
