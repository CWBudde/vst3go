#include "program.h"

#include <string.h>

static Steinberg_tresult SMTG_STDMETHODCALLTYPE programs_query(void* self, const Steinberg_TUID iid, void** obj) {
    Steinberg_FUnknown* owner = ((ProgramInfoInterface*)self)->owner;
    return owner->lpVtbl->queryInterface(owner, iid, obj);
}

static Steinberg_uint32 SMTG_STDMETHODCALLTYPE programs_add_ref(void* self) {
    Steinberg_FUnknown* owner = ((ProgramInfoInterface*)self)->owner;
    return owner->lpVtbl->addRef(owner);
}

static Steinberg_uint32 SMTG_STDMETHODCALLTYPE programs_release(void* self) {
    Steinberg_FUnknown* owner = ((ProgramInfoInterface*)self)->owner;
    return owner->lpVtbl->release(owner);
}

static Steinberg_int32 SMTG_STDMETHODCALLTYPE programs_unit_count(void* self) {
    return 1;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE programs_unit_info(void* self, Steinberg_int32 index,
                                                               struct Steinberg_Vst_UnitInfo* info) {
    if (index != 0 || !info) {
        return Steinberg_kInvalidArgument;
    }
    struct Steinberg_Vst_ProgramListInfo list;
    Steinberg_tresult result = GoFactoryProgramListInfo(((ProgramInfoInterface*)self)->componentID, &list);
    if (result != Steinberg_kResultOk) {
        return result;
    }
    memset(info, 0, sizeof(*info));
    info->id = Steinberg_Vst_kRootUnitId;
    info->parentUnitId = Steinberg_Vst_kNoParentUnitId;
    info->programListId = list.id;
    memcpy(info->name, list.name, sizeof(info->name));
    return Steinberg_kResultOk;
}

static Steinberg_int32 SMTG_STDMETHODCALLTYPE programs_list_count(void* self) {
    return GoFactoryProgramListCount(((ProgramInfoInterface*)self)->componentID);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE programs_list_info(void* self, Steinberg_int32 index,
                                                                  struct Steinberg_Vst_ProgramListInfo* info) {
    if (index != 0 || !info) {
        return Steinberg_kInvalidArgument;
    }
    return GoFactoryProgramListInfo(((ProgramInfoInterface*)self)->componentID, info);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE programs_name(void* self, Steinberg_Vst_ProgramListID list,
                                                             Steinberg_int32 program, Steinberg_Vst_String128 name) {
    if (!name) {
        return Steinberg_kInvalidArgument;
    }
    return GoFactoryProgramName(((ProgramInfoInterface*)self)->componentID, list, program, name);
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE programs_attribute(void* self, Steinberg_Vst_ProgramListID list,
    Steinberg_int32 program, Steinberg_Vst_CString attribute, Steinberg_Vst_String128 value) {
    return Steinberg_kResultFalse;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE programs_has_pitch_names(void* self, Steinberg_Vst_ProgramListID list,
                                                                       Steinberg_int32 program) {
    return Steinberg_kResultFalse;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE programs_pitch_name(void* self, Steinberg_Vst_ProgramListID list,
    Steinberg_int32 program, Steinberg_int16 pitch, Steinberg_Vst_String128 name) {
    return Steinberg_kResultFalse;
}

static Steinberg_Vst_UnitID SMTG_STDMETHODCALLTYPE programs_selected_unit(void* self) {
    return Steinberg_Vst_kRootUnitId;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE programs_select_unit(void* self, Steinberg_Vst_UnitID unit) {
    return unit == Steinberg_Vst_kRootUnitId ? Steinberg_kResultOk : Steinberg_kInvalidArgument;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE programs_unit_by_bus(void* self, Steinberg_Vst_MediaType media,
    Steinberg_Vst_BusDirection direction, Steinberg_int32 bus, Steinberg_int32 channel, Steinberg_Vst_UnitID* unit) {
    return Steinberg_kResultFalse;
}

static Steinberg_tresult SMTG_STDMETHODCALLTYPE programs_set_data(void* self, Steinberg_int32 list,
                                                               Steinberg_int32 program, Steinberg_IBStream* data) {
    return Steinberg_kNotImplemented;
}

static Steinberg_Vst_IUnitInfoVtbl programs_vtable = {
    .queryInterface = programs_query,
    .addRef = programs_add_ref,
    .release = programs_release,
    .getUnitCount = programs_unit_count,
    .getUnitInfo = programs_unit_info,
    .getProgramListCount = programs_list_count,
    .getProgramListInfo = programs_list_info,
    .getProgramName = programs_name,
    .getProgramInfo = programs_attribute,
    .hasProgramPitchNames = programs_has_pitch_names,
    .getProgramPitchName = programs_pitch_name,
    .getSelectedUnit = programs_selected_unit,
    .selectUnit = programs_select_unit,
    .getUnitByBus = programs_unit_by_bus,
    .setUnitProgramData = programs_set_data,
};

void initProgramInfo(ProgramInfoInterface* unit, Steinberg_FUnknown* owner, uintptr_t componentID) {
    unit->iface.lpVtbl = &programs_vtable;
    unit->owner = owner;
    unit->componentID = componentID;
}
