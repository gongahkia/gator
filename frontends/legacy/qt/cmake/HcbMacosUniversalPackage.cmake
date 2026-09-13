function(hcb_validate_macos_universal_architectures architectures)
  set(hcb_expected_architectures arm64 x86_64)
  set(hcb_actual_architectures ${architectures})
  list(REMOVE_DUPLICATES hcb_actual_architectures)
  list(SORT hcb_actual_architectures)
  if(NOT hcb_actual_architectures STREQUAL hcb_expected_architectures)
    message(FATAL_ERROR
      "macOS universal packaging requires exactly arm64;x86_64, got: ${architectures}"
    )
  endif()
endfunction()

function(hcb_configure_macos_universal_package target)
  if(NOT APPLE)
    message(FATAL_ERROR "macOS universal packaging is only supported on macOS hosts")
  endif()
  if(NOT TARGET ${target})
    message(FATAL_ERROR "macOS universal package target does not exist: ${target}")
  endif()
  hcb_validate_macos_universal_architectures("${CMAKE_OSX_ARCHITECTURES}")

  set_target_properties(${target} PROPERTIES OSX_ARCHITECTURES "arm64;x86_64")
  install(TARGETS ${target} BUNDLE DESTINATION .)
  qt_generate_deploy_qml_app_script(
    TARGET ${target}
    OUTPUT_SCRIPT hcb_macos_universal_deploy_script
  )
  install(SCRIPT "${hcb_macos_universal_deploy_script}")
  install(CODE [=[
set(hcb_bundle_root "$ENV{DESTDIR}${CMAKE_INSTALL_PREFIX}/Hot Cross Buns.app")
set(hcb_bundle_executable "${hcb_bundle_root}/Contents/MacOS/Hot Cross Buns")
if(NOT EXISTS "${hcb_bundle_executable}")
  message(FATAL_ERROR "macOS package is missing the native executable")
endif()
file(GLOB_RECURSE hcb_bundle_files LIST_DIRECTORIES FALSE "${hcb_bundle_root}/*")
foreach(hcb_bundle_file IN LISTS hcb_bundle_files)
  execute_process(
    COMMAND /usr/bin/file -b "${hcb_bundle_file}"
    RESULT_VARIABLE hcb_file_result
    OUTPUT_VARIABLE hcb_file_description
    ERROR_VARIABLE hcb_file_error
  )
  if(NOT hcb_file_result EQUAL 0)
    message(FATAL_ERROR "Unable to inspect packaged file ${hcb_bundle_file}: ${hcb_file_error}")
  endif()
  if(hcb_file_description MATCHES "Mach-O")
    execute_process(
      COMMAND /usr/bin/lipo -verify_arch arm64 x86_64 "${hcb_bundle_file}"
      RESULT_VARIABLE hcb_lipo_result
      ERROR_VARIABLE hcb_lipo_error
    )
    if(NOT hcb_lipo_result EQUAL 0)
      message(FATAL_ERROR "Packaged Mach-O is not universal: ${hcb_bundle_file}: ${hcb_lipo_error}")
    endif()
  endif()
endforeach()
]=])

  set(CPACK_GENERATOR DragNDrop PARENT_SCOPE)
  set(CPACK_PACKAGE_FILE_NAME "Hot-Cross-Buns-${PROJECT_VERSION}-macos-universal" PARENT_SCOPE)
  set(CPACK_DMG_VOLUME_NAME "Hot Cross Buns" PARENT_SCOPE)
  set(CPACK_PACKAGE_CHECKSUM SHA256 PARENT_SCOPE)
endfunction()
