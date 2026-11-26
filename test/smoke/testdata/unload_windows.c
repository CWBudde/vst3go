#include <stdbool.h>
#include <stdio.h>
#include <string.h>
#include <windows.h>
#include "vst3/vst3_c_api.h"

typedef bool (*ModuleEntry)(void);
typedef struct Steinberg_IPluginFactory* (*GetFactory)(void);

static bool query_factory(HMODULE module) {
    GetFactory get_factory = (GetFactory)(void*)GetProcAddress(module, "GetPluginFactory");
    struct Steinberg_IPluginFactory* factory = get_factory ? get_factory() : NULL;
    if (!factory)
        return false;
    int count = factory->lpVtbl->countClasses(factory);
    factory->lpVtbl->release(factory);
    return count > 0;
}

int main(int argc, char** argv) {
    if (argc != 3)
        return 1;

    HMODULE module = LoadLibraryA(argv[1]);
    if (!module) {
        fprintf(stderr, "LoadLibrary failed (%lu)\n", GetLastError());
        return 1;
    }

    if (strcmp(argv[2], "init") == 0) {
        ModuleEntry init = (ModuleEntry)(void*)GetProcAddress(module, "InitDll");
        ModuleEntry exit = (ModuleEntry)(void*)GetProcAddress(module, "ExitDll");
        if (!init || !init() || !exit || !exit())
            return 1;
    } else if (!query_factory(module)) {
        return 1;
    }

    if (!FreeLibrary(module))
        return 1;
    HMODULE resident = GetModuleHandleA(argv[1]);
    if (resident != module) {
        fprintf(stderr, "FreeLibrary unmapped the Go module after %s\n", argv[2]);
        return 1;
    }

    HMODULE reloaded = LoadLibraryA(argv[1]);
    if (reloaded != resident || !query_factory(reloaded) || !FreeLibrary(reloaded))
        return 1;

    puts("ALL PASSED");
    return 0;
}
