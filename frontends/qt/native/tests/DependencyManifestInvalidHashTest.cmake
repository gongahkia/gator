cmake_minimum_required(VERSION 3.28)

if(NOT DEFINED HCB_SOURCE_DIR OR NOT DEFINED HCB_MANIFEST)
  message(FATAL_ERROR "HCB_SOURCE_DIR and HCB_MANIFEST are required")
endif()

execute_process(
  COMMAND ${CMAKE_COMMAND}
    "-DHCB_SOURCE_DIR=${HCB_SOURCE_DIR}"
    "-DHCB_MANIFEST=${HCB_MANIFEST}"
    -P "${HCB_SOURCE_DIR}/native/tests/DependencyManifestTest.cmake"
  RESULT_VARIABLE hcb_result
  OUTPUT_VARIABLE hcb_output
  ERROR_VARIABLE hcb_error
)
if(hcb_result EQUAL 0)
  message(FATAL_ERROR "Invalid dependency manifest was accepted")
endif()
if(NOT "${hcb_output}${hcb_error}" MATCHES "invalid SHA256 hash")
  message(FATAL_ERROR "Invalid dependency manifest failed for the wrong reason")
endif()
