#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

[Setup]
AppId={{B8A4E3D1-5F7A-4C9B-A6D2-8E1F3C5B7A90}
AppName=ClipShot
AppVersion={#AppVersion}
AppPublisher=Lapius7
AppPublisherURL=https://github.com/Lapius7/clipshot-app
DefaultDirName={autopf}\ClipShot
DefaultGroupName=ClipShot
OutputDir=installer
OutputBaseFilename=clipshot-setup-{#AppVersion}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
DisableProgramGroupPage=yes
LicenseFile=LICENSE

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "japanese"; MessagesFile: "compiler:Languages\Japanese.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "clipshot.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\ClipShot"; Filename: "{app}\clipshot.exe"
Name: "{group}\{cm:UninstallProgram,ClipShot}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\ClipShot"; Filename: "{app}\clipshot.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\clipshot.exe"; Description: "{cm:LaunchProgram,ClipShot}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
Type: filesandordirs; Name: "{app}"
