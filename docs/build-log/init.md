
# 1. 项目初始化

已按计划完成初始化，摘要如下。

## Go 后端

- 根目录 `go.mod`：module loomidbx，依赖 github.com/json-iterator/go。
- `backend/cmd/main.go`：LDB_Version（返回 JSON）、LDB_FreeString，供 c-shared 导出。
- 已建立 steering 中的子包占位：connector、schema、generator（含 int/、string/、decimal/、datetime/、boolean/）、writer、storage、ffi。
- 语言限制：int、string 不能作为包名，目录仍叫 int/string，包名分别为 intgen、strgen（见对应 doc.go）。

编译输出目录：`backend/build/libldb.dll`（及同目录生成的 .h）。一键脚本：`scripts/build-go-windows.ps1`。

## Flutter 前端

- frontend/：flutter create 含 windows / macos / linux。
- `pubspec.yaml`：flutter_riverpod；主题色与 steering 主色一致（#2563EB）。
- `lib/main.dart`：ProviderScope + LoomiDBXApp。
- 已建目录：ffi/（native_library.dart 预留 DynamicLibrary.open）、models/、screens/、widgets/、providers/。
- Windows 资源：`windows/runner/Runner.rc` 中公司名改为 com.aierdong.loomidbx。
- 为兼容当前环境（Flutter 3.3 / Dart 2.18），未使用 ColorScheme.fromSeed / Material 3 等较新 API。

## 仓库

已扩展 `.gitignore`（backend/build/、**/build/、.dart_tool/ 等）。

## 已执行验证

| 命令                                                                    |	结果 |
| :--------------------------------------------------------------------- | :--- |
| go build -buildmode=c-shared -o backend/build/libldb.dll ./backend/cmd | 通过  |
| go test ./...                                                          | 通过  |
| flutter analyze                                                        | 无问题 |
| flutter test                                                           | 通过  |
| flutter build windows                                                  | 通过  |

本地可执行：`cd frontend` 后 `flutter run -d windows`（需本机 Flutter 在 PATH 或使用 D:\flutter\bin\flutter.bat）。
DLL 联调时请将 backend/build/libldb.dll 放到与 loomidbx.exe 同目录或按 native_library.dart 调整加载路径。
所有相关 to-do 已勾选完成。
若你希望下一步把 CMake 里自动复制 libldb.dll 到 runner 输出目录，可以说一下我按 Windows 流程补上。
