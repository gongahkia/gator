if(NOT DEFINED HCB_SOURCE_DIR)
  message(FATAL_ERROR "HCB_SOURCE_DIR is required")
endif()

include("${HCB_SOURCE_DIR}/cmake/HcbMacosUniversalPackage.cmake")

if(HCB_EXPECT_INVALID_ARCHITECTURES)
  hcb_validate_macos_universal_architectures("arm64")
  message(FATAL_ERROR "invalid universal architecture set was accepted")
endif()

hcb_validate_macos_universal_architectures("arm64;x86_64")
execute_process(
  COMMAND "${CMAKE_COMMAND}"
    "-DHCB_SOURCE_DIR=${HCB_SOURCE_DIR}"
    -DHCB_EXPECT_INVALID_ARCHITECTURES=ON
    -P "${CMAKE_CURRENT_LIST_FILE}"
  RESULT_VARIABLE hcb_invalid_result
)
if(hcb_invalid_result EQUAL 0)
  message(FATAL_ERROR "invalid universal architecture set did not fail")
endif()

file(READ "${HCB_SOURCE_DIR}/CMakePresets.json" hcb_presets)
if(NOT hcb_presets MATCHES "\"name\": \"macos-universal-package\"")
  message(FATAL_ERROR "macOS universal package preset is missing")
endif()
if(NOT hcb_presets MATCHES "\"CMAKE_OSX_ARCHITECTURES\": \"arm64;x86_64\"")
  message(FATAL_ERROR "macOS universal package preset does not declare both architectures")
endif()
