cmake_minimum_required(VERSION 3.21)
project(SetupUI LANGUAGES CXX)

find_package(Qt6 6.5 REQUIRED COMPONENTS Widgets)
qt_add_executable(Setup-UI WIN32 MACOSX_BUNDLE main.cpp)
if(APPLE)
    find_library(CORE_GRAPHICS_FRAMEWORK CoreGraphics REQUIRED)
    find_library(CORE_FOUNDATION_FRAMEWORK CoreFoundation REQUIRED)
    target_link_libraries(Setup-UI PRIVATE ${CORE_GRAPHICS_FRAMEWORK} ${CORE_FOUNDATION_FRAMEWORK})
    set_target_properties(Setup-UI PROPERTIES
        MACOSX_BUNDLE_BUNDLE_NAME "SimCity 4 UHD Setup"
        MACOSX_BUNDLE_ICON_FILE "setup.icns"
        MACOSX_BUNDLE_GUI_IDENTIFIER "io.github.feverqwe.simcity4uhd.setup")
endif()
target_compile_features(Setup-UI PRIVATE cxx_std_23)
target_link_libraries(Setup-UI PRIVATE Qt6::Widgets)
install(TARGETS Setup-UI RUNTIME DESTINATION qt BUNDLE DESTINATION qt)
