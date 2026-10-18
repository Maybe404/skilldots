import * as React from "react"
import { basicSetup } from "codemirror"
import { Compartment, EditorState, type Extension } from "@codemirror/state"
import { EditorView } from "@codemirror/view"
import { HighlightStyle, StreamLanguage, syntaxHighlighting, type LanguageSupport } from "@codemirror/language"
import { markdown } from "@codemirror/lang-markdown"
import { python } from "@codemirror/lang-python"
import { javascript } from "@codemirror/lang-javascript"
import { json } from "@codemirror/lang-json"
import { yaml } from "@codemirror/lang-yaml"
import { shell } from "@codemirror/legacy-modes/mode/shell"
import { tags as t } from "@lezer/highlight"

/** Language name shown in the status bar, by file extension. */
export function languageName(path: string): string | null {
  const ext = path.split(".").pop()?.toLowerCase() ?? ""
  return (
    {
      md: "Markdown",
      markdown: "Markdown",
      py: "Python",
      sh: "Shell",
      bash: "Shell",
      zsh: "Shell",
      js: "JavaScript",
      mjs: "JavaScript",
      cjs: "JavaScript",
      jsx: "JavaScript",
      ts: "TypeScript",
      tsx: "TypeScript",
      json: "JSON",
      yaml: "YAML",
      yml: "YAML",
    }[ext] ?? null
  )
}

const shellLang = () => StreamLanguage.define(shell)

function fenceLanguage(info: string) {
  switch (info.toLowerCase()) {
    case "py":
    case "python":
      return python().language
    case "sh":
    case "bash":
    case "shell":
    case "zsh":
      return shellLang()
    case "js":
    case "javascript":
    case "jsx":
      return javascript({ jsx: true }).language
    case "ts":
    case "typescript":
    case "tsx":
      return javascript({ typescript: true, jsx: true }).language
    case "json":
      return json().language
    case "yaml":
    case "yml":
      return yaml().language
    default:
      return null
  }
}

function languageFor(path: string): Extension {
  switch (languageName(path)) {
    case "Markdown":
      return markdown({ codeLanguages: fenceLanguage })
    case "Python":
      return python()
    case "Shell":
      return shellLang()
    case "JavaScript":
      return javascript({ jsx: true })
    case "TypeScript":
      return javascript({ typescript: true, jsx: path.endsWith("x") })
    case "JSON":
      return json()
    case "YAML":
      return yaml()
    default:
      return [] as unknown as LanguageSupport
  }
}

/*
 * Graphite highlighting: one hue, so syntax is told apart by weight, style
 * and lightness only (foreground / muted / faint), never by colour.
 */
const graphiteHighlight = HighlightStyle.define([
  { tag: [t.keyword, t.controlKeyword, t.moduleKeyword, t.operatorKeyword, t.definitionKeyword], fontWeight: "600" },
  { tag: [t.heading1, t.heading2], fontWeight: "700" },
  { tag: [t.heading, t.heading3, t.heading4, t.heading5, t.heading6], fontWeight: "650" },
  { tag: [t.strong], fontWeight: "700" },
  { tag: [t.emphasis], fontStyle: "italic" },
  { tag: [t.strikethrough], textDecoration: "line-through" },
  { tag: [t.link, t.url], textDecoration: "underline", textUnderlineOffset: "3px" },
  { tag: [t.string, t.special(t.string), t.regexp, t.monospace], color: "var(--muted-foreground)" },
  { tag: [t.comment, t.lineComment, t.blockComment, t.docComment], color: "var(--faint)", fontStyle: "italic" },
  { tag: [t.meta, t.processingInstruction, t.contentSeparator, t.quote], color: "var(--muted-foreground)" },
  { tag: [t.number, t.bool, t.null, t.atom], fontWeight: "550" },
  { tag: [t.function(t.variableName), t.function(t.propertyName), t.className, t.typeName], fontWeight: "550" },
  { tag: [t.propertyName, t.attributeName], color: "var(--foreground)" },
  { tag: [t.punctuation, t.bracket, t.separator, t.operator], color: "var(--muted-foreground)" },
  { tag: [t.list], color: "var(--muted-foreground)" },
  { tag: [t.invalid], textDecoration: "underline wavy" },
])

function graphiteTheme(dark: boolean) {
  return EditorView.theme(
    {
      "&": {
        backgroundColor: "var(--background)",
        color: "var(--foreground)",
        fontSize: "13px", fontFamily: "var(--fmCode)",
        height: "100%",
      },
      ".cm-scroller": {
        fontFamily: "var(--fmCode)",
        lineHeight: "1.65",
      },
      ".cm-content": { caretColor: "var(--foreground)", padding: "12px 0" },
      ".cm-line": { padding: "0 16px 0 12px" },
      "&.cm-focused": { outline: "none" },
      ".cm-cursor, .cm-dropCursor": { borderLeftColor: "var(--foreground)", borderLeftWidth: "1.5px" },
      "&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, ::selection": {
        backgroundColor: "color-mix(in oklch, var(--foreground) 16%, transparent) !important",
      },
      ".cm-activeLine": { backgroundColor: "color-mix(in oklch, var(--foreground) 4%, transparent)" },
      ".cm-gutters": {
        backgroundColor: "var(--background)",
        color: "var(--faint)",
        border: "none",
        fontFamily: "var(--fmCode)",
      },
      ".cm-activeLineGutter": { backgroundColor: "transparent", color: "var(--foreground)" },
      ".cm-lineNumbers .cm-gutterElement": { padding: "0 4px 0 16px", minWidth: "36px" },
      ".cm-foldGutter .cm-gutterElement": { color: "var(--faint)" },
      ".cm-matchingBracket, .cm-nonmatchingBracket": {
        backgroundColor: "color-mix(in oklch, var(--foreground) 12%, transparent)",
        outline: "none",
      },
      ".cm-searchMatch": {
        backgroundColor: "color-mix(in oklch, var(--foreground) 14%, transparent)",
        outline: "1px solid color-mix(in oklch, var(--foreground) 35%, transparent)",
      },
      ".cm-searchMatch.cm-searchMatch-selected": { backgroundColor: "color-mix(in oklch, var(--foreground) 26%, transparent)" },
      ".cm-selectionMatch": { backgroundColor: "color-mix(in oklch, var(--foreground) 9%, transparent)" },
      ".cm-panels": { backgroundColor: "var(--muted)", color: "var(--foreground)", borderColor: "var(--border)" },
      ".cm-panels input, .cm-panels button": { fontFamily: "inherit" },
      ".cm-tooltip": {
        backgroundColor: "var(--popover)",
        color: "var(--popover-foreground)",
        border: "1px solid var(--border)",
        borderRadius: "8px",
      },
      ".cm-tooltip-autocomplete > ul > li[aria-selected]": {
        backgroundColor: "var(--primary)",
        color: "var(--primary-foreground)",
      },
      ".cm-foldPlaceholder": { backgroundColor: "var(--muted)", border: "none", color: "var(--muted-foreground)" },
    },
    { dark },
  )
}

export interface CodeEditorHandle {
  focus: () => void
}

export const CodeEditor = React.forwardRef<
  CodeEditorHandle,
  {
    /** Identity of the document; a new id resets the editor state and history. */
    docId: string
    path: string
    value: string
    onChange: (value: string) => void
    onCursor?: (line: number, col: number) => void
    readOnly?: boolean
    dark: boolean
    label: string
  }
>(function CodeEditor({ docId, path, value, onChange, onCursor, readOnly = false, dark, label }, ref) {
  const host = React.useRef<HTMLDivElement>(null)
  const view = React.useRef<EditorView | null>(null)
  const theme = React.useRef(new Compartment())
  const ro = React.useRef(new Compartment())
  const handlers = React.useRef({ onChange, onCursor })
  handlers.current = { onChange, onCursor }

  React.useImperativeHandle(ref, () => ({ focus: () => view.current?.focus() }), [])

  // (Re)create the view when the document changes.
  React.useEffect(() => {
    if (!host.current) return
    const v = new EditorView({
      parent: host.current,
      state: EditorState.create({
        doc: value,
        extensions: [
          basicSetup,
          EditorView.lineWrapping,
          languageFor(path),
          syntaxHighlighting(graphiteHighlight),
          theme.current.of(graphiteTheme(dark)),
          ro.current.of([EditorState.readOnly.of(readOnly), EditorView.editable.of(!readOnly)]),
          EditorView.contentAttributes.of({ "aria-label": label }),
          EditorView.updateListener.of((u) => {
            if (u.docChanged) handlers.current.onChange(u.state.doc.toString())
            if (u.selectionSet || u.docChanged) {
              const head = u.state.selection.main.head
              const line = u.state.doc.lineAt(head)
              handlers.current.onCursor?.(line.number, head - line.from + 1)
            }
          }),
        ],
      }),
    })
    view.current = v
    return () => {
      v.destroy()
      view.current = null
    }
    // value is only the initial content; later changes come from the editor itself
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [docId, path])

  // Text changed outside the editor (e.g. the description form rewrote the frontmatter).
  React.useEffect(() => {
    const v = view.current
    if (v && v.state.doc.toString() !== value) {
      v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: value } })
    }
  }, [value])

  React.useEffect(() => {
    view.current?.dispatch({ effects: theme.current.reconfigure(graphiteTheme(dark)) })
  }, [dark])
  React.useEffect(() => {
    view.current?.dispatch({ effects: ro.current.reconfigure([EditorState.readOnly.of(readOnly), EditorView.editable.of(!readOnly)]) })
  }, [readOnly])

  return <div ref={host} className="h-full min-h-0 overflow-hidden [&_.cm-editor]:h-full" />
})
