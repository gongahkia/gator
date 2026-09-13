option(HCB_ENABLE_FORMAT_CHECK "Add the native format-check target" OFF)
set(HCB_CLANG_FORMAT_EXECUTABLE "" CACHE FILEPATH "Path to clang-format")

function(hcb_find_clang_format)
  if(HCB_CLANG_FORMAT_EXECUTABLE)
    if(NOT EXISTS "${HCB_CLANG_FORMAT_EXECUTABLE}")
      message(FATAL_ERROR "HCB_CLANG_FORMAT_EXECUTABLE does not exist: ${HCB_CLANG_FORMAT_EXECUTABLE}")
    endif()
    return()
  endif()

  if(APPLE)
    set(hcb_clang_format_hints /opt/homebrew/opt/llvm/bin /usr/local/opt/llvm/bin)
  endif()
  unset(HCB_CLANG_FORMAT_EXECUTABLE CACHE)
  find_program(HCB_CLANG_FORMAT_EXECUTABLE
    NAMES clang-format clang-format-22
    HINTS ${hcb_clang_format_hints}
  )
  if(NOT HCB_CLANG_FORMAT_EXECUTABLE)
    message(FATAL_ERROR "clang-format is required when HCB_ENABLE_FORMAT_CHECK=ON")
  endif()
endfunction()

function(hcb_add_format_check_target)
  if(NOT HCB_ENABLE_FORMAT_CHECK)
    return()
  endif()

  hcb_find_clang_format()
  file(GLOB_RECURSE hcb_format_sources CONFIGURE_DEPENDS LIST_DIRECTORIES FALSE
    "${PROJECT_SOURCE_DIR}/native/src/*.cpp"
    "${PROJECT_SOURCE_DIR}/native/src/*.h"
    "${PROJECT_SOURCE_DIR}/native/tests/*.cpp"
    "${PROJECT_SOURCE_DIR}/native/tests/*.h"
  )
  add_custom_target(hcb_format_check
    COMMAND ${HCB_CLANG_FORMAT_EXECUTABLE} --dry-run --Werror ${hcb_format_sources}
    COMMAND_EXPAND_LISTS
    VERBATIM
  )
endfunction()
