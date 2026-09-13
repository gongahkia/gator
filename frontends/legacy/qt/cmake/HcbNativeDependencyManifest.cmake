function(hcb_native_dependency_manifest_get json artifact_index output_variable)
  string(JSON hcb_value ERROR_VARIABLE hcb_json_error GET
    "${json}" artifacts ${artifact_index} ${ARGN}
  )
  if(NOT hcb_json_error STREQUAL "NOTFOUND")
    message(FATAL_ERROR
      "Native dependency manifest artifact ${artifact_index} is missing ${ARGN}: ${hcb_json_error}"
    )
  endif()
  set(${output_variable} "${hcb_value}" PARENT_SCOPE)
endfunction()

function(hcb_native_dependency_manifest_require_hash artifact_id algorithm value)
  if(NOT "${algorithm}" STREQUAL "SHA256" AND NOT "${algorithm}" STREQUAL "SHA3_256")
    message(FATAL_ERROR
      "Native dependency manifest artifact ${artifact_id} has unsupported hash algorithm: ${algorithm}"
    )
  endif()
  string(LENGTH "${value}" hcb_hash_length)
  if(NOT hcb_hash_length EQUAL 64 OR NOT "${value}" MATCHES "^[0-9a-f]+$")
    message(FATAL_ERROR
      "Native dependency manifest artifact ${artifact_id} has an invalid ${algorithm} hash"
    )
  endif()
endfunction()

function(hcb_native_dependency_manifest_require_value artifact_id property value)
  if("${value}" STREQUAL "")
    message(FATAL_ERROR
      "Native dependency manifest artifact ${artifact_id} has an empty ${property}"
    )
  endif()
endfunction()

function(hcb_load_native_dependency_manifest manifest_path)
  get_filename_component(hcb_manifest_path "${manifest_path}" ABSOLUTE
    BASE_DIR "${PROJECT_SOURCE_DIR}"
  )
  if(NOT EXISTS "${hcb_manifest_path}")
    message(FATAL_ERROR "Native dependency manifest does not exist: ${hcb_manifest_path}")
  endif()

  file(READ "${hcb_manifest_path}" hcb_manifest_json)
  string(JSON hcb_schema_version ERROR_VARIABLE hcb_schema_error GET
    "${hcb_manifest_json}" schema_version
  )
  if(NOT hcb_schema_error STREQUAL "NOTFOUND" OR NOT hcb_schema_version EQUAL 1)
    message(FATAL_ERROR "Native dependency manifest must use schema_version 1")
  endif()

  string(JSON hcb_artifact_count ERROR_VARIABLE hcb_artifact_count_error LENGTH
    "${hcb_manifest_json}" artifacts
  )
  if(NOT hcb_artifact_count_error STREQUAL "NOTFOUND" OR NOT hcb_artifact_count EQUAL 2)
    message(FATAL_ERROR "Native dependency manifest must define exactly two artifacts")
  endif()

  set(hcb_seen_artifacts)
  math(EXPR hcb_last_artifact_index "${hcb_artifact_count} - 1")
  foreach(hcb_artifact_index RANGE 0 ${hcb_last_artifact_index})
    hcb_native_dependency_manifest_get("${hcb_manifest_json}" ${hcb_artifact_index} hcb_id id)
    hcb_native_dependency_manifest_get("${hcb_manifest_json}" ${hcb_artifact_index} hcb_version version)
    hcb_native_dependency_manifest_get("${hcb_manifest_json}" ${hcb_artifact_index} hcb_license license)
    hcb_native_dependency_manifest_get("${hcb_manifest_json}" ${hcb_artifact_index} hcb_source_directory
      source_directory
    )
    hcb_native_dependency_manifest_get("${hcb_manifest_json}" ${hcb_artifact_index} hcb_archive_url
      archive url
    )
    hcb_native_dependency_manifest_get("${hcb_manifest_json}" ${hcb_artifact_index} hcb_archive_filename
      archive filename
    )
    hcb_native_dependency_manifest_get("${hcb_manifest_json}" ${hcb_artifact_index} hcb_archive_size_bytes
      archive size_bytes
    )
    hcb_native_dependency_manifest_get("${hcb_manifest_json}" ${hcb_artifact_index} hcb_hash_algorithm
      archive hash algorithm
    )
    hcb_native_dependency_manifest_get("${hcb_manifest_json}" ${hcb_artifact_index} hcb_hash_value
      archive hash value
    )

    hcb_native_dependency_manifest_require_value("${hcb_id}" version "${hcb_version}")
    hcb_native_dependency_manifest_require_value("${hcb_id}" license "${hcb_license}")
    hcb_native_dependency_manifest_require_value("${hcb_id}" source_directory "${hcb_source_directory}")
    hcb_native_dependency_manifest_require_value("${hcb_id}" archive.filename "${hcb_archive_filename}")
    if(NOT "${hcb_id}" MATCHES "^[a-z][a-z0-9-]*$")
      message(FATAL_ERROR "Native dependency manifest has invalid artifact id: ${hcb_id}")
    endif()
    if("${hcb_id}" IN_LIST hcb_seen_artifacts)
      message(FATAL_ERROR "Native dependency manifest repeats artifact id: ${hcb_id}")
    endif()
    if(NOT "${hcb_version}" MATCHES "^[0-9]+\\.[0-9]+\\.[0-9]+$")
      message(FATAL_ERROR "Native dependency manifest artifact ${hcb_id} has invalid version: ${hcb_version}")
    endif()
    if(NOT "${hcb_archive_url}" MATCHES "^https://" OR "${hcb_archive_url}" MATCHES "[?#]")
      message(FATAL_ERROR "Native dependency manifest artifact ${hcb_id} has an invalid archive URL")
    endif()
    if(NOT "${hcb_archive_size_bytes}" MATCHES "^[1-9][0-9]*$")
      message(FATAL_ERROR "Native dependency manifest artifact ${hcb_id} has invalid archive.size_bytes")
    endif()
    if("${hcb_archive_filename}" MATCHES "[/\\\\]")
      message(FATAL_ERROR "Native dependency manifest artifact ${hcb_id} has invalid archive.filename")
    endif()
    hcb_native_dependency_manifest_require_hash("${hcb_id}" "${hcb_hash_algorithm}" "${hcb_hash_value}")

    list(APPEND hcb_seen_artifacts "${hcb_id}")
    if("${hcb_id}" STREQUAL "qt")
      if(NOT "${hcb_archive_url}" MATCHES "^https://download\\.qt\\.io/official_releases/qt/")
        message(FATAL_ERROR "Qt must use the official Qt release archive")
      endif()
      if(NOT "${hcb_archive_filename}" STREQUAL "qt-everywhere-src-${hcb_version}.tar.xz")
        message(FATAL_ERROR "Qt archive.filename must match the pinned version")
      endif()
      if(NOT "${hcb_source_directory}" STREQUAL "qt-everywhere-src-${hcb_version}")
        message(FATAL_ERROR "Qt source_directory must match the pinned version")
      endif()
      string(REPLACE "." ";" hcb_qt_version_parts "${hcb_version}")
      list(GET hcb_qt_version_parts 0 hcb_qt_major)
      list(GET hcb_qt_version_parts 1 hcb_qt_minor)
      set(hcb_expected_qt_url
        "https://download.qt.io/official_releases/qt/${hcb_qt_major}.${hcb_qt_minor}/${hcb_version}/single/${hcb_archive_filename}"
      )
      if(NOT "${hcb_archive_url}" STREQUAL "${hcb_expected_qt_url}")
        message(FATAL_ERROR "Qt archive URL must match the pinned version and filename")
      endif()
      set(hcb_qt_version "${hcb_version}")
      set(hcb_qt_archive_url "${hcb_archive_url}")
      set(hcb_qt_archive_filename "${hcb_archive_filename}")
      set(hcb_qt_archive_size_bytes "${hcb_archive_size_bytes}")
      set(hcb_qt_hash_algorithm "${hcb_hash_algorithm}")
      set(hcb_qt_hash_value "${hcb_hash_value}")
      set(hcb_qt_source_directory "${hcb_source_directory}")
    elseif("${hcb_id}" STREQUAL "sqlite")
      if(NOT "${hcb_archive_url}" MATCHES "^https://www\\.sqlite\\.org/[0-9][0-9][0-9][0-9]/")
        message(FATAL_ERROR "SQLite must use the official SQLite release archive")
      endif()
      if(NOT "${hcb_archive_filename}" MATCHES "^sqlite-amalgamation-[0-9]+\\.zip$")
        message(FATAL_ERROR "SQLite archive.filename must be an amalgamation archive")
      endif()
      if(NOT "${hcb_source_directory}" MATCHES "^sqlite-amalgamation-[0-9]+$")
        message(FATAL_ERROR "SQLite source_directory must be an amalgamation directory")
      endif()
      string(REGEX REPLACE "\\.zip$" "" hcb_expected_sqlite_source_directory "${hcb_archive_filename}")
      if(NOT "${hcb_source_directory}" STREQUAL "${hcb_expected_sqlite_source_directory}")
        message(FATAL_ERROR "SQLite source_directory must match archive.filename")
      endif()
      if(NOT "${hcb_archive_url}" MATCHES "/${hcb_archive_filename}$")
        message(FATAL_ERROR "SQLite archive URL must end with archive.filename")
      endif()
      set(hcb_sqlite_version "${hcb_version}")
      set(hcb_sqlite_archive_url "${hcb_archive_url}")
      set(hcb_sqlite_archive_filename "${hcb_archive_filename}")
      set(hcb_sqlite_archive_size_bytes "${hcb_archive_size_bytes}")
      set(hcb_sqlite_hash_algorithm "${hcb_hash_algorithm}")
      set(hcb_sqlite_hash_value "${hcb_hash_value}")
      set(hcb_sqlite_source_directory "${hcb_source_directory}")
    else()
      message(FATAL_ERROR "Native dependency manifest has unsupported artifact id: ${hcb_id}")
    endif()
  endforeach()

  foreach(hcb_required_artifact IN ITEMS qt sqlite)
    if(NOT "${hcb_required_artifact}" IN_LIST hcb_seen_artifacts)
      message(FATAL_ERROR "Native dependency manifest is missing ${hcb_required_artifact}")
    endif()
  endforeach()

  set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS "${hcb_manifest_path}")
  set(HCB_NATIVE_DEPENDENCY_MANIFEST_PATH "${hcb_manifest_path}" PARENT_SCOPE)
  set(HCB_NATIVE_QT_VERSION "${hcb_qt_version}" PARENT_SCOPE)
  set(HCB_NATIVE_QT_ARCHIVE_URL "${hcb_qt_archive_url}" PARENT_SCOPE)
  set(HCB_NATIVE_QT_ARCHIVE_FILENAME "${hcb_qt_archive_filename}" PARENT_SCOPE)
  set(HCB_NATIVE_QT_ARCHIVE_SIZE_BYTES "${hcb_qt_archive_size_bytes}" PARENT_SCOPE)
  set(HCB_NATIVE_QT_HASH_ALGORITHM "${hcb_qt_hash_algorithm}" PARENT_SCOPE)
  set(HCB_NATIVE_QT_HASH_VALUE "${hcb_qt_hash_value}" PARENT_SCOPE)
  set(HCB_NATIVE_QT_SOURCE_DIRECTORY "${hcb_qt_source_directory}" PARENT_SCOPE)
  set(HCB_NATIVE_SQLITE_VERSION "${hcb_sqlite_version}" PARENT_SCOPE)
  set(HCB_NATIVE_SQLITE_ARCHIVE_URL "${hcb_sqlite_archive_url}" PARENT_SCOPE)
  set(HCB_NATIVE_SQLITE_ARCHIVE_FILENAME "${hcb_sqlite_archive_filename}" PARENT_SCOPE)
  set(HCB_NATIVE_SQLITE_ARCHIVE_SIZE_BYTES "${hcb_sqlite_archive_size_bytes}" PARENT_SCOPE)
  set(HCB_NATIVE_SQLITE_HASH_ALGORITHM "${hcb_sqlite_hash_algorithm}" PARENT_SCOPE)
  set(HCB_NATIVE_SQLITE_HASH_VALUE "${hcb_sqlite_hash_value}" PARENT_SCOPE)
  set(HCB_NATIVE_SQLITE_SOURCE_DIRECTORY "${hcb_sqlite_source_directory}" PARENT_SCOPE)

  message(STATUS "Native dependency manifest: Qt ${hcb_qt_version}, SQLite ${hcb_sqlite_version}")
endfunction()
