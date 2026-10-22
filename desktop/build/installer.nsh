; Stop only the reviewed login supervisor owned by this installation. Preserve
; user data. An update keeps the Run value; a full uninstall removes this exact
; command without touching a different Gator installation or profile.
!macro customUnInstall
  ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "GatorBackgroundChecks"
  StrCmp $0 '"$INSTDIR\resources\runtime\gator-runtime.exe" job supervisor --login-profile' 0 gator_uninstall_done
  nsExec::ExecToLog '"$INSTDIR\resources\runtime\gator-runtime.exe" job supervisor --login-profile --stop'
  ${ifNot} ${isUpdated}
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "GatorBackgroundChecks"
  ${endIf}
gator_uninstall_done:
!macroend
