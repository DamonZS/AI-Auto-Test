[Setup]
AppId=Auto-Test.Desktop
AppName=Auto-Test
AppVersion=0.1.2
DefaultDirName={localappdata}\Programs\Auto-Test
DefaultGroupName=Auto-Test
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputBaseFilename=Auto-Test-windows-x64-setup
Compression=lzma2/fast
SolidCompression=yes
SetupIconFile={#Payload}\app-icon.ico
UninstallDisplayIcon={app}\autotest-company.ico
ShowLanguageDialog=no

[Languages]
Name: "zhcn"; MessagesFile: "ChineseSimplified.isl"

[Files]
Source: "{#Payload}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#Payload}\app-icon.ico"; DestDir: "{app}"; DestName: "autotest-company.ico"; Flags: ignoreversion

[InstallDelete]
Type: files; Name: "{group}\Stop Auto-Test.lnk"

[Icons]
Name: "{group}\Auto-Test"; Filename: "{app}\python\pythonw.exe"; Parameters: """{app}\desktop.py"""; WorkingDir: "{app}"; IconFilename: "{app}\autotest-company.ico"; AppUserModelID: "AutoTest.Desktop"
Name: "{group}\停止 Auto-Test"; Filename: "{app}\python\pythonw.exe"; Parameters: """{app}\desktop.py"" --stop --notify"; WorkingDir: "{app}"; IconFilename: "{app}\autotest-company.ico"

[Run]
Filename: "{app}\python\pythonw.exe"; Parameters: """{app}\desktop.py"""; Description: "打开 Auto-Test"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{app}\python\pythonw.exe"; Parameters: """{app}\desktop.py"" --stop"; RunOnceId: "StopDesktop"; Flags: runhidden

[Code]
function CreateFile(FileName: String; Access, ShareMode, Security, Creation, Flags, Template: LongWord): LongWord;
  external 'CreateFileW@kernel32.dll stdcall';
function CloseHandle(Handle: LongWord): Boolean;
  external 'CloseHandle@kernel32.dll stdcall';

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  LockPath, Launcher, Runtime: String;
  LockHandle: LongWord;
  ExitCode: Integer;
begin
  Result := '';
  Launcher := ExpandConstant('{app}\desktop.py');
  Runtime := ExpandConstant('{app}\python\pythonw.exe');
  LockPath := AddBackslash(GetEnv('LOCALAPPDATA')) + 'Auto-Test\desktop.lock';
  if not (FileExists(Launcher) and FileExists(Runtime) and FileExists(LockPath)) then
    Exit;

  LockHandle := CreateFile(LockPath, $80000000, 0, 0, 3, 0, 0);
  if LockHandle <> $FFFFFFFF then
  begin
    CloseHandle(LockHandle);
    Exit;
  end;

  if not WizardSilent then
    if TaskDialogMsgBox('Auto-Test 正在运行',
      '升级需要关闭旧版 Auto-Test。当前正在执行的测试将中断。',
      mbConfirmation, MB_OKCANCEL, ['关闭后继续', '取消安装'], 0) <> IDOK then
    begin
      Result := '安装已取消。';
      Exit;
    end;

  WizardForm.StatusLabel.Caption := '正在关闭旧版 Auto-Test…';
  if not Exec(Runtime, '"' + Launcher + '" --stop', ExpandConstant('{app}'),
    SW_HIDE, ewWaitUntilTerminated, ExitCode) or (ExitCode <> 0) then
    Result := '无法关闭旧版 Auto-Test。请从托盘退出后重新运行安装程序。';
end;
