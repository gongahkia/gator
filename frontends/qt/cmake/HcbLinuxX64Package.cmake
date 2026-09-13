function(hcb_validate_linux_x64_architecture architecture)
  if(NOT architecture STREQUAL "x86_64" AND NOT architecture STREQUAL "AMD64")
    message(FATAL_ERROR "Linux x64 packaging requires x86_64, got: ${architecture}")
  endif()
endfunction()

function(hcb_configure_linux_x64_package target)
  if(NOT CMAKE_SYSTEM_NAME STREQUAL "Linux")
    message(FATAL_ERROR "Linux x64 packaging is only supported on Linux hosts")
  endif()
  if(CMAKE_VERSION VERSION_LESS "4.2")
    message(FATAL_ERROR "Linux AppImage packaging requires CMake 4.2 or newer")
  endif()
  if(NOT TARGET ${target})
    message(FATAL_ERROR "Linux x64 package target does not exist: ${target}")
  endif()
  hcb_validate_linux_x64_architecture("${CMAKE_SYSTEM_PROCESSOR}")

  set_target_properties(${target} PROPERTIES OUTPUT_NAME hot-cross-buns)
  set(hcb_desktop_file "${CMAKE_BINARY_DIR}/hot-cross-buns.desktop")
  configure_file(
    "${PROJECT_SOURCE_DIR}/native/packaging/hot-cross-buns.desktop.in"
    "${hcb_desktop_file}"
    @ONLY
  )
  install(TARGETS ${target} RUNTIME DESTINATION bin)
  install(FILES "${hcb_desktop_file}" DESTINATION share/applications)
  install(FILES "${PROJECT_SOURCE_DIR}/assets/brand/app-icon.png"
    DESTINATION share/icons/hicolor/1024x1024/apps
    RENAME hot-cross-buns.png
  )
  qt_generate_deploy_qml_app_script(
    TARGET ${target}
    OUTPUT_SCRIPT hcb_linux_x64_deploy_script
  )
  install(SCRIPT "${hcb_linux_x64_deploy_script}")
  install(CODE [=[
set(hcb_executable "$ENV{DESTDIR}${CMAKE_INSTALL_PREFIX}/bin/hot-cross-buns")
if(NOT EXISTS "${hcb_executable}")
  message(FATAL_ERROR "Linux package is missing the native executable")
endif()
file(GET_RUNTIME_DEPENDENCIES
  EXECUTABLES "${hcb_executable}"
  RESOLVED_DEPENDENCIES_VAR hcb_runtime_dependencies
  UNRESOLVED_DEPENDENCIES_VAR hcb_unresolved_dependencies
)
if(hcb_unresolved_dependencies)
  message(FATAL_ERROR "Linux package has unresolved runtime dependencies: ${hcb_unresolved_dependencies}")
endif()
foreach(hcb_dependency IN LISTS hcb_runtime_dependencies)
  if(hcb_dependency MATCHES "/Qt[^/]*\\.so")
    file(INSTALL
      DESTINATION "$ENV{DESTDIR}${CMAKE_INSTALL_PREFIX}/lib"
      TYPE SHARED_LIBRARY
      FOLLOW_SYMLINK_CHAIN
      FILES "${hcb_dependency}"
    )
  endif()
endforeach()
]=])

  set(CPACK_GENERATOR AppImage PARENT_SCOPE)
  set(CPACK_PACKAGE_FILE_NAME "Hot-Cross-Buns-${PROJECT_VERSION}-linux-x64" PARENT_SCOPE)
  set(CPACK_PACKAGE_ICON hot-cross-buns PARENT_SCOPE)
  set(CPACK_APPIMAGE_DESKTOP_FILE hot-cross-buns.desktop PARENT_SCOPE)
  set(CPACK_APPIMAGE_NO_APPSTREAM TRUE PARENT_SCOPE)
  set(CPACK_PACKAGE_CHECKSUM SHA256 PARENT_SCOPE)
endfunction()
