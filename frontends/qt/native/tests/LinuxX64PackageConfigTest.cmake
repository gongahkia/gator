if(NOT DEFINED HCB_SOURCE_DIR)
  message(FATAL_ERROR "HCB_SOURCE_DIR is required")
endif()

include("${HCB_SOURCE_DIR}/cmake/HcbLinuxX64Package.cmake")

if(HCB_EXPECT_INVALID_ARCHITECTURE)
  hcb_validate_linux_x64_architecture("aarch64")
  message(FATAL_ERROR "invalid Linux architecture was accepted")
endif()

hcb_validate_linux_x64_architecture("x86_64")
hcb_validate_linux_x64_architecture("AMD64")
execute_process(
  COMMAND "${CMAKE_COMMAND}"
    "-DHCB_SOURCE_DIR=${HCB_SOURCE_DIR}"
    -DHCB_EXPECT_INVALID_ARCHITECTURE=ON
    -P "${CMAKE_CURRENT_LIST_FILE}"
  RESULT_VARIABLE hcb_invalid_result
)
if(hcb_invalid_result EQUAL 0)
  message(FATAL_ERROR "invalid Linux architecture did not fail")
endif()

file(READ "${HCB_SOURCE_DIR}/CMakePresets.json" hcb_presets)
if(NOT hcb_presets MATCHES "\"name\": \"linux-x64-package\"")
  message(FATAL_ERROR "Linux x64 package preset is missing")
endif()
file(READ "${HCB_SOURCE_DIR}/native/packaging/hot-cross-buns.desktop.in" hcb_desktop_file)
if(NOT hcb_desktop_file MATCHES "Exec=hot-cross-buns")
  message(FATAL_ERROR "Linux desktop entry does not target the packaged executable")
endif()
