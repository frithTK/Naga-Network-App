#ifndef RepoRoot
  #error RepoRoot must be passed as /DRepoRoot=...
#endif
#ifndef MyAppVersion
  #define MyAppVersion "0.0.0"
#endif

#define MyAppName "Naga Network"
#define MyAppPublisher "Naga Network"
#define MyAppExeName "NagaNetwork.exe"

[Setup]
AppId={{A7E3C4B1-9D2F-4E8A-B5C1-6F0A2D8E91C3}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppMutex=Local\NagaNetworkUI
DefaultDirName={localappdata}\Programs\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableDirPage=auto
DisableProgramGroupPage=yes
PrivilegesRequired=admin
UsedUserAreasWarning=no
UsePreviousAppDir=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
SourceDir={#RepoRoot}
OutputDir={#RepoRoot}\dist\windows
OutputBaseFilename=NagaNetwork-{#MyAppVersion}-windows-x64-setup
SetupIconFile={#RepoRoot}\assets\brand\naga-network.ico
UninstallDisplayIcon={app}\{#MyAppExeName}
WizardStyle=modern
Compression=lzma2
SolidCompression=yes
CloseApplications=yes
CloseApplicationsFilter=NagaNetwork.exe,naga_network.exe,naga-control.exe,sing-box.exe,olcrtc.exe,hev-socks5-tunnel.exe
RestartApplications=no

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"

[CustomMessages]
english.DeleteUserData=Also delete profiles and settings stored in %LOCALAPPDATA%\naga-network?
russian.DeleteUserData=Удалить также профили и настройки из %LOCALAPPDATA%\naga-network?
english.CloseNagaFailed=Could not stop Naga Network. Close the connection and try again.
russian.CloseNagaFailed=Не удалось закрыть Naga Network. Закрой подключение и повтори.
english.InstallHelperFailed=Could not register the administrator helper. TUN connect will still ask for UAC.
russian.InstallHelperFailed=Не удалось зарегистрировать helper с правами администратора. «Подключить» в TUN по-прежнему запросит UAC.

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "dist\windows\NagaNetwork\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; WorkingDir: "{app}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; WorkingDir: "{app}"; Flags: nowait postinstall skipifsilent runasoriginaluser

[Registry]
Root: HKCU; Subkey: "Software\Classes\nagavpn"; ValueType: string; ValueName: ""; ValueData: "URL:Naga Network"; Flags: uninsdeletekey
Root: HKCU; Subkey: "Software\Classes\nagavpn"; ValueType: string; ValueName: "URL Protocol"; ValueData: ""
Root: HKCU; Subkey: "Software\Classes\nagavpn\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\{#MyAppExeName}"" ""%1"""

[Code]
var
  DeleteUserData: Boolean;

function NagaProcessStillRunning(const ImageName: String): Boolean;
var
  Locator: Variant;
  Service: Variant;
  Processes: Variant;
begin
  Result := False;
  try
    Locator := CreateOleObject('WbemScripting.SWbemLocator');
    Service := Locator.ConnectServer('.', 'root\CIMV2');
    Processes := Service.ExecQuery(
      'SELECT Name FROM Win32_Process WHERE Name=''' + ImageName + '''');
    Result := (not VarIsNull(Processes)) and (Processes.Count > 0);
  except
    Result := False;
  end;
end;

procedure KillNagaImage(const ImageName: String);
var
  ResultCode: Integer;
begin
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM ' + ImageName, '',
    SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

function KillNagaProcesses: Boolean;
var
  I: Integer;
begin
  KillNagaImage('naga_network.exe');
  KillNagaImage('NagaNetwork.exe');
  KillNagaImage('naga-control.exe');
  for I := 1 to 8 do
  begin
    Sleep(500);
    if (not NagaProcessStillRunning('naga-control.exe')) and
       (not NagaProcessStillRunning('sing-box.exe')) then
    begin
      Result := True;
      exit;
    end;
  end;
  KillNagaImage('sing-box.exe');
  Sleep(1000);
  Result := (not NagaProcessStillRunning('naga-control.exe')) and
            (not NagaProcessStillRunning('sing-box.exe'));
end;

function RemoveElevatedHelper: Boolean;
var
  ResultCode: Integer;
begin
  Result := Exec(ExpandConstant('{app}\naga-control.exe'), '--uninstall-helper',
    ExpandConstant('{app}'), SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

function InstallElevatedHelper: Boolean;
var
  ResultCode: Integer;
begin
  Result := Exec(ExpandConstant('{app}\naga-control.exe'), '--install-helper',
    ExpandConstant('{app}'), SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Result := Result and (ResultCode = 0);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  NeedsRestart := False;
  Result := '';
  if not KillNagaProcesses then
    Result := CustomMessage('CloseNagaFailed');
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    if not InstallElevatedHelper then
      MsgBox(CustomMessage('InstallHelperFailed'), mbInformation, MB_OK);
    RegDeleteKeyIncludingSubkeys(HKCU,
      'Software\Microsoft\Windows\CurrentVersion\Uninstall\{A7E3C4B1-9D2F-4E8A-B5C1-6F0A2D8E91C3}_is1');
  end;
end;

function InitializeUninstall: Boolean;
begin
  Result := True;
  DeleteUserData := False;
  RemoveElevatedHelper;
  if not KillNagaProcesses then
  begin
    MsgBox(CustomMessage('CloseNagaFailed'), mbError, MB_OK);
    Result := False;
    exit;
  end;
  if UninstallSilent then
    exit;
  DeleteUserData := MsgBox(CustomMessage('DeleteUserData'), mbConfirmation,
    MB_YESNO or MB_DEFBUTTON2) = IDYES;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if (CurUninstallStep = usPostUninstall) and DeleteUserData then
    DelTree(ExpandConstant('{localappdata}\naga-network'), True, True, True);
end;
