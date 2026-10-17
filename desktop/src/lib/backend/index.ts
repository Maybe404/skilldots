import { isMyGo } from "mygo-runtime"
import { createMockBackend } from "./mock"
import { nativeBackend } from "./native"
import type { Backend } from "./types"

export * from "./types"

// Browser preview uses sample data. Native windows always use the Go service;
// a failed native call never falls back to a successful mock operation.
export const backend: Backend = isMyGo() ? nativeBackend : createMockBackend()
