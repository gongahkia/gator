include(FetchContent)

function(hcb_add_sqlite_library)
  if(TARGET hcb_sqlite)
    return()
  endif()
  if(NOT "${FETCHCONTENT_SOURCE_DIR_HCB_SQLITE}" STREQUAL "")
    message(FATAL_ERROR "FETCHCONTENT_SOURCE_DIR_HCB_SQLITE bypasses the pinned SQLite artifact")
  endif()

  set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)
  FetchContent_Declare(hcb_sqlite
    URL "${HCB_NATIVE_SQLITE_ARCHIVE_URL}"
    URL_HASH "${HCB_NATIVE_SQLITE_HASH_ALGORITHM}=${HCB_NATIVE_SQLITE_HASH_VALUE}"
    SOURCE_SUBDIR hcb-no-cmake-project
    DOWNLOAD_EXTRACT_TIMESTAMP TRUE
  )
  FetchContent_MakeAvailable(hcb_sqlite)

  set(hcb_sqlite_source "${hcb_sqlite_SOURCE_DIR}/sqlite3.c")
  set(hcb_sqlite_header "${hcb_sqlite_SOURCE_DIR}/sqlite3.h")
  if(NOT EXISTS "${hcb_sqlite_source}" OR NOT EXISTS "${hcb_sqlite_header}")
    message(FATAL_ERROR "Pinned SQLite archive is missing sqlite3.c or sqlite3.h")
  endif()

  find_package(Threads REQUIRED)
  add_library(hcb_sqlite STATIC "${hcb_sqlite_source}" "${hcb_sqlite_header}")
  add_library(hcb::sqlite ALIAS hcb_sqlite)
  set_target_properties(hcb_sqlite PROPERTIES
    C_STANDARD 99
    C_STANDARD_REQUIRED ON
    C_EXTENSIONS OFF
    POSITION_INDEPENDENT_CODE ON
  )
  target_compile_definitions(hcb_sqlite PRIVATE SQLITE_THREADSAFE=1 SQLITE_ENABLE_FTS5=1)
  target_include_directories(hcb_sqlite SYSTEM PUBLIC "${hcb_sqlite_SOURCE_DIR}")
  target_link_libraries(hcb_sqlite PUBLIC Threads::Threads ${CMAKE_DL_LIBS})
endfunction()
