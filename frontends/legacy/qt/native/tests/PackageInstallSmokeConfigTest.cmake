if(NOT DEFINED HCB_SOURCE_DIR)
  message(FATAL_ERROR "HCB_SOURCE_DIR is required")
endif()

set(hcb_required_files
  native/packaging/smoke/macos-install-smoke.sh
  native/packaging/smoke/linux-install-smoke.sh
  native/packaging/smoke/windows-install-smoke.ps1
  .github/workflows/native-package-install-smoke.yml
)
foreach(hcb_required_file IN LISTS hcb_required_files)
  if(NOT EXISTS "${HCB_SOURCE_DIR}/${hcb_required_file}")
    message(FATAL_ERROR "package install smoke file is missing: ${hcb_required_file}")
  endif()
endforeach()

file(READ "${HCB_SOURCE_DIR}/.github/workflows/native-package-install-smoke.yml" hcb_workflow)
foreach(hcb_job IN ITEMS
    native-macos-package-install-smoke
    native-linux-package-install-smoke
    native-windows-package-install-smoke
)
  if(NOT hcb_workflow MATCHES "${hcb_job}:")
    message(FATAL_ERROR "package install smoke job is missing: ${hcb_job}")
  endif()
endforeach()
if(hcb_workflow MATCHES "pnpm|Electron|electron")
  message(FATAL_ERROR "package install smoke must not depend on Electron")
endif()
