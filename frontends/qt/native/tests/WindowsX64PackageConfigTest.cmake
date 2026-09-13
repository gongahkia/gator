if(NOT DEFINED HCB_SOURCE_DIR)
  message(FATAL_ERROR "HCB_SOURCE_DIR is required")
endif()

include("${HCB_SOURCE_DIR}/cmake/HcbWindowsX64Package.cmake")

if(HCB_EXPECT_INVALID_ARCHITECTURE)
  hcb_validate_windows_x64_architecture("ARM64" 8)
  message(FATAL_ERROR "invalid Windows architecture was accepted")
endif()

if(HCB_EXPECT_INVALID_POINTER_SIZE)
  hcb_validate_windows_x64_architecture("AMD64" 4)
  message(FATAL_ERROR "invalid Windows pointer size was accepted")
endif()

hcb_validate_windows_x64_architecture("AMD64" 8)
hcb_validate_windows_x64_architecture("x86_64" 8)
execute_process(
  COMMAND "${CMAKE_COMMAND}"
    "-DHCB_SOURCE_DIR=${HCB_SOURCE_DIR}"
    -DHCB_EXPECT_INVALID_ARCHITECTURE=ON
    -P "${CMAKE_CURRENT_LIST_FILE}"
  RESULT_VARIABLE hcb_invalid_architecture_result
)
if(hcb_invalid_architecture_result EQUAL 0)
  message(FATAL_ERROR "invalid Windows architecture did not fail")
endif()
execute_process(
  COMMAND "${CMAKE_COMMAND}"
    "-DHCB_SOURCE_DIR=${HCB_SOURCE_DIR}"
    -DHCB_EXPECT_INVALID_POINTER_SIZE=ON
    -P "${CMAKE_CURRENT_LIST_FILE}"
  RESULT_VARIABLE hcb_invalid_pointer_size_result
)
if(hcb_invalid_pointer_size_result EQUAL 0)
  message(FATAL_ERROR "invalid Windows pointer size did not fail")
endif()

file(READ "${HCB_SOURCE_DIR}/CMakePresets.json" hcb_presets)
if(NOT hcb_presets MATCHES "\"name\": \"windows-x64-package\"")
  message(FATAL_ERROR "Windows x64 package preset is missing")
endif()
if(NOT EXISTS "${HCB_SOURCE_DIR}/build/icon.ico")
  message(FATAL_ERROR "Windows installer icon is missing")
endif()
