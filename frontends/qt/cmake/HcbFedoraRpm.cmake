include(GNUInstallDirs)

function(hcb_configure_fedora_rpm application_target reminder_target)
  if(NOT CMAKE_SYSTEM_NAME STREQUAL "Linux")
    message(FATAL_ERROR "Fedora RPM packaging is only supported on Linux hosts")
  endif()
  if(NOT HCB_NATIVE_DEPENDENCY_MODE_RESOLVED STREQUAL "system")
    message(FATAL_ERROR "Fedora RPM packaging requires HCB_NATIVE_DEPENDENCY_MODE=system")
  endif()
  if(NOT TARGET ${application_target} OR NOT TARGET ${reminder_target})
    message(FATAL_ERROR "Fedora RPM packaging requires the application and reminder daemon targets")
  endif()

  set_target_properties(${application_target} PROPERTIES OUTPUT_NAME hot-cross-buns)
  set_target_properties(${reminder_target} PROPERTIES OUTPUT_NAME hcb-reminderd)
  set(hcb_desktop_file "${CMAKE_BINARY_DIR}/hot-cross-buns.desktop")
  configure_file(
    "${PROJECT_SOURCE_DIR}/native/packaging/hot-cross-buns.desktop.in"
    "${hcb_desktop_file}"
    @ONLY
  )

  install(TARGETS ${application_target} RUNTIME DESTINATION "${CMAKE_INSTALL_BINDIR}")
  install(TARGETS ${reminder_target}
    RUNTIME DESTINATION "${CMAKE_INSTALL_LIBEXECDIR}/hot-cross-buns"
  )
  install(FILES "${hcb_desktop_file}" DESTINATION "${CMAKE_INSTALL_DATAROOTDIR}/applications")
  install(FILES "${PROJECT_SOURCE_DIR}/assets/brand/app-icon.png"
    DESTINATION "${CMAKE_INSTALL_DATAROOTDIR}/icons/hicolor/1024x1024/apps"
    RENAME hot-cross-buns.png
  )
  install(FILES "${PROJECT_SOURCE_DIR}/native/packaging/hcb-reminderd.service"
    DESTINATION "${CMAKE_INSTALL_LIBDIR}/systemd/user"
  )
endfunction()
