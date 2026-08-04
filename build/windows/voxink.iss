#ifndef AppVersion
  #error AppVersion must be supplied with ISCC /DAppVersion=<version>
#endif

[Setup]
AppId={{53B77ED9-6929-4BF1-8487-4A34C5A07163}
AppName=VoxInk
AppVersion={#AppVersion}
DefaultDirName={localappdata}\Programs\VoxInk
DefaultGroupName=VoxInk
PrivilegesRequired=lowest
ArchitecturesAllowed=x64
ArchitecturesInstallIn64BitMode=x64
DisableProgramGroupPage=yes
OutputDir={#SourcePath}\..\..\dist
OutputBaseFilename=voxink-windows-amd64-setup
SourceDir={#SourcePath}\..\..\dist\bundle
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\voxink.exe

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Additional shortcuts:"; Flags: unchecked

[Files]
Source: "voxink.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "voxink-cli.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "THIRD_PARTY_NOTICES.md"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\VoxInk"; Filename: "{app}\voxink.exe"; WorkingDir: "{app}"
Name: "{group}\VoxInk CLI"; Filename: "{cmd}"; WorkingDir: "{app}"; IconFilename: "{app}\voxink-cli.exe"
Name: "{autodesktop}\VoxInk"; Filename: "{app}\voxink.exe"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
Filename: "{app}\voxink.exe"; Description: "Launch VoxInk"; Flags: nowait postinstall skipifsilent

; Deliberately omit UninstallDelete entries: uninstall removes installed program
; files and shortcuts but preserves %AppData%\VoxInk and Credential Manager data.
