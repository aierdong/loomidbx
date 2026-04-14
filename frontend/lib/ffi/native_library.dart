import 'dart:ffi';
import 'dart:io';

/// Loads the Go shared library. Wire [LDB_FreeString] / JSON calls when FFI contract is ready.
///
/// Windows: ship `libldb.dll` next to the executable (see `backend/build/`).
DynamicLibrary? loadLdbLibrary() {
  if (Platform.isWindows) {
    return DynamicLibrary.open('libldb.dll');
  }
  if (Platform.isLinux) {
    return DynamicLibrary.open('libldb.so');
  }
  if (Platform.isMacOS) {
    return DynamicLibrary.open('libldb.dylib');
  }
  return null;
}
