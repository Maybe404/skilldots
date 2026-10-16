import { createMockBackend } from "./mock"
import type { Backend } from "./types"

export * from "./types"

/**
 * The backend the app uses. Swap this for the generated Go bindings once the
 * desktop shell exists; the UI only depends on the Backend interface.
 */
export const backend: Backend = createMockBackend()
