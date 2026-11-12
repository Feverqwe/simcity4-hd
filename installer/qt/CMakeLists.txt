cmake_minimum_required(VERSION 3.21)
project(SetupUI LANGUAGES CXX)

find_package(Qt6 6.5 REQUIRED COMPONENTS Widgets)
qt_add_executable(Setup-UI WIN32 main.cpp)
target_compile_features(Setup-UI PRIVATE cxx_std_17)
target_link_libraries(Setup-UI PRIVATE Qt6::Widgets)
install(TARGETS Setup-UI RUNTIME DESTINATION qt)
