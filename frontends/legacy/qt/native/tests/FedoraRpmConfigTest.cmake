if(NOT DEFINED HCB_SOURCE_DIR)
  message(FATAL_ERROR "HCB_SOURCE_DIR is required")
endif()

set(hcb_required_files
  cmake/HcbFedoraRpm.cmake
  packaging/fedora/hot-cross-buns.spec
  native/packaging/hcb-reminderd.service
  native/src/reminder/ReminderDaemonMain.cpp
)
foreach(hcb_required_file IN LISTS hcb_required_files)
  if(NOT EXISTS "${HCB_SOURCE_DIR}/${hcb_required_file}")
    message(FATAL_ERROR "Fedora RPM file is missing: ${hcb_required_file}")
  endif()
endforeach()

file(READ "${HCB_SOURCE_DIR}/CMakePresets.json" hcb_presets)
foreach(hcb_preset IN ITEMS fedora43-debug fedora43-rpm)
  if(NOT hcb_presets MATCHES "\"name\": \"${hcb_preset}\"")
    message(FATAL_ERROR "Fedora preset is missing: ${hcb_preset}")
  endif()
endforeach()

file(READ "${HCB_SOURCE_DIR}/native/packaging/hot-cross-buns.desktop.in" hcb_desktop_file)
foreach(hcb_required_key IN ITEMS
    "Exec=hot-cross-buns %u"
    "MimeType=x-scheme-handler/hotcrossbuns;"
)
  if(NOT hcb_desktop_file MATCHES "${hcb_required_key}")
    message(FATAL_ERROR "Fedora desktop entry is missing: ${hcb_required_key}")
  endif()
endforeach()

file(READ "${HCB_SOURCE_DIR}/packaging/fedora/hot-cross-buns.spec" hcb_spec)
foreach(hcb_required_spec_value IN ITEMS
    "HCB_NATIVE_DEPENDENCY_MODE=system"
    "HCB_ENABLE_FEDORA_RPM=ON"
    "qt6-qtbase-devel"
    "qt6-qtdeclarative-devel"
    "sqlite-devel"
    "%{_userunitdir}/hcb-reminderd.service"
)
  if(NOT hcb_spec MATCHES "${hcb_required_spec_value}")
    message(FATAL_ERROR "Fedora RPM spec is missing: ${hcb_required_spec_value}")
  endif()
endforeach()
