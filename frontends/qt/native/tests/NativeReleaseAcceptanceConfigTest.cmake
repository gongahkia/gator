if(NOT DEFINED HCB_SOURCE_DIR)
  message(FATAL_ERROR "HCB_SOURCE_DIR is required")
endif()

set(hcb_workflow_path "${HCB_SOURCE_DIR}/.github/workflows/native-release-acceptance.yml")
if(NOT EXISTS "${hcb_workflow_path}")
  message(FATAL_ERROR "native release acceptance workflow is missing")
endif()

file(READ "${hcb_workflow_path}" hcb_workflow)
foreach(hcb_required_pattern IN ITEMS
    "workflow_dispatch:"
    "tags:"
    "- \"v\\*\""
    "native-package-install-smoke.yml"
    "native-macos-live-google-attestation"
    "check-macos-live-google-acceptance.sh"
    "macos-live-google-acceptance-v1.json"
    "needs: native-macos-live-google-attestation"
)
  if(NOT hcb_workflow MATCHES "${hcb_required_pattern}")
    message(FATAL_ERROR "native release acceptance workflow is missing: ${hcb_required_pattern}")
  endif()
endforeach()
if(hcb_workflow MATCHES "pnpm|Electron|electron")
  message(FATAL_ERROR "native release acceptance must not depend on Electron")
endif()
