option(HCB_ENABLE_BOOTSTRAP_CHECK "Add the developer bootstrap verification target" OFF)

function(hcb_add_bootstrap_check)
  if(NOT HCB_ENABLE_BOOTSTRAP_CHECK)
    return()
  endif()
  if(NOT BUILD_TESTING)
    message(FATAL_ERROR "Developer bootstrap verification requires BUILD_TESTING=ON")
  endif()

  set(hcb_ctest_configuration)
  if(CMAKE_CONFIGURATION_TYPES)
    set(hcb_ctest_configuration -C $<CONFIG>)
  endif()
  add_custom_target(hcb_bootstrap_check
    COMMAND ${CMAKE_CTEST_COMMAND}
      --test-dir ${CMAKE_BINARY_DIR}
      ${hcb_ctest_configuration}
      --output-on-failure
    DEPENDS ${ARGN}
    COMMAND_EXPAND_LISTS
    USES_TERMINAL
    VERBATIM
  )
endfunction()
