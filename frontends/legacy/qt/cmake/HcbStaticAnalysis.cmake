option(HCB_ENABLE_STATIC_ANALYSIS "Run clang-tidy while compiling native targets" OFF)
set(HCB_CLANG_TIDY_EXECUTABLE "" CACHE FILEPATH "Path to clang-tidy")

function(hcb_find_clang_tidy)
  if(HCB_CLANG_TIDY_EXECUTABLE)
    if(NOT EXISTS "${HCB_CLANG_TIDY_EXECUTABLE}")
      message(FATAL_ERROR "HCB_CLANG_TIDY_EXECUTABLE does not exist: ${HCB_CLANG_TIDY_EXECUTABLE}")
    endif()
    return()
  endif()

  if(APPLE)
    set(hcb_clang_tidy_hints /opt/homebrew/opt/llvm/bin /usr/local/opt/llvm/bin)
  endif()
  unset(HCB_CLANG_TIDY_EXECUTABLE CACHE)
  find_program(HCB_CLANG_TIDY_EXECUTABLE
    NAMES clang-tidy clang-tidy-22
    HINTS ${hcb_clang_tidy_hints}
  )
  if(NOT HCB_CLANG_TIDY_EXECUTABLE)
    message(FATAL_ERROR "clang-tidy is required when HCB_ENABLE_STATIC_ANALYSIS=ON")
  endif()
endfunction()

function(hcb_add_static_analysis_target)
  if(NOT HCB_ENABLE_STATIC_ANALYSIS)
    return()
  endif()

  if(NOT CMAKE_EXPORT_COMPILE_COMMANDS)
    message(FATAL_ERROR "Static analysis requires CMAKE_EXPORT_COMPILE_COMMANDS=ON")
  endif()
  hcb_find_clang_tidy()
  set(hcb_clang_tidy_arguments
    --config-file=${PROJECT_SOURCE_DIR}/.clang-tidy
    --warnings-as-errors=*
  )
  if(APPLE)
    if(CMAKE_OSX_SYSROOT)
      set(hcb_macos_sdk "${CMAKE_OSX_SYSROOT}")
    else()
      execute_process(
        COMMAND xcrun --sdk macosx --show-sdk-path
        OUTPUT_VARIABLE hcb_macos_sdk
        OUTPUT_STRIP_TRAILING_WHITESPACE
        COMMAND_ERROR_IS_FATAL ANY
      )
    endif()
    list(APPEND hcb_clang_tidy_arguments
      --extra-arg=-isysroot
      --extra-arg=${hcb_macos_sdk}
    )
  endif()
  set(hcb_analysis_globs "${PROJECT_SOURCE_DIR}/native/src/*.cpp")
  if(BUILD_TESTING)
    list(APPEND hcb_analysis_globs "${PROJECT_SOURCE_DIR}/native/tests/*.cpp")
  endif()
  file(GLOB_RECURSE hcb_analysis_sources CONFIGURE_DEPENDS LIST_DIRECTORIES FALSE
    ${hcb_analysis_globs}
  )
  add_custom_target(hcb_static_analysis
    COMMAND ${HCB_CLANG_TIDY_EXECUTABLE}
      ${hcb_clang_tidy_arguments}
      -p ${CMAKE_BINARY_DIR}
      ${hcb_analysis_sources}
    DEPENDS ${ARGN}
    COMMAND_EXPAND_LISTS
    VERBATIM
  )
endfunction()
