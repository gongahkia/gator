if(NOT DEFINED HCB_SOURCE_DIR)
  message(FATAL_ERROR "HCB_SOURCE_DIR is required")
endif()

set(hcb_removed_paths
  bin
  scripts
  src
  tests
  electron-builder.yml
  electron.vite.config.ts
  package.json
  playwright.config.ts
  pnpm-lock.yaml
  postcss.config.cjs
  tailwind.config.ts
  tsconfig.browser-extension.json
  tsconfig.json
  vitest.config.ts
  vitest.setup.ts
)
foreach(hcb_removed_path IN LISTS hcb_removed_paths)
  if(EXISTS "${HCB_SOURCE_DIR}/${hcb_removed_path}")
    message(FATAL_ERROR "Electron cutover path remains: ${hcb_removed_path}")
  endif()
endforeach()

file(READ "${HCB_SOURCE_DIR}/Makefile" hcb_makefile)
if(hcb_makefile MATCHES "pnpm|electron")
  message(FATAL_ERROR "Makefile retains an Electron command")
endif()

file(GLOB hcb_workflows "${HCB_SOURCE_DIR}/.github/workflows/*.yml")
foreach(hcb_workflow_path IN LISTS hcb_workflows)
  file(READ "${hcb_workflow_path}" hcb_workflow)
  if(hcb_workflow MATCHES "pnpm|Electron|electron")
    message(FATAL_ERROR "workflow retains an Electron dependency: ${hcb_workflow_path}")
  endif()
endforeach()
