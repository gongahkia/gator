option(HCB_ENABLE_SANITIZERS "Enable supported runtime sanitizers" OFF)
set(HCB_SANITIZERS "address;undefined" CACHE STRING "Semicolon-separated native sanitizers")

function(hcb_enable_sanitizers target)
  if(NOT HCB_ENABLE_SANITIZERS)
    return()
  endif()

  if(MSVC)
    if(NOT "address" IN_LIST HCB_SANITIZERS)
      message(FATAL_ERROR "MSVC sanitizer builds require HCB_SANITIZERS to include address")
    endif()
    target_compile_options(${target} PRIVATE /fsanitize=address)
    target_link_options(${target} PRIVATE /INCREMENTAL:NO)
    return()
  endif()

  string(JOIN "," hcb_sanitizer_flags ${HCB_SANITIZERS})
  target_compile_options(${target} PRIVATE
    "-fsanitize=${hcb_sanitizer_flags}"
    -fno-omit-frame-pointer
    -g
  )
  target_link_options(${target} PRIVATE "-fsanitize=${hcb_sanitizer_flags}")
endfunction()
