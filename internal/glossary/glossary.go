// Package glossary holds the user-facing definitions of Voxi's audio words,
// shared by the help texts of `voxi chunks`, `voxi sample` and `voxi voice`.
// The reference is docs/SampleStore.md §1.
package glossary

// Audio explains chunk, sample, purpose, publish, merge and voice.
const Audio = `Terms:
  chunk    one recording cut by live dictation, with its ASR diagnostics; only the
           last 100 are kept, in $XDG_RUNTIME_DIR (cleared on reboot) when available
  sample   a saved recording with its exact transcript, stored by purpose under
           ~/.local/share/voxi/samples/{dictation,noise,voice}
  purpose  dictation (ASR accuracy checks), noise (must yield no text; the only
           publishable purpose) or voice (your own voice, for cloning and training;
           needs your consent)
  publish  copy a noise sample into the repository's public store (testdata/samples)
  merge    join samples or chunks into one longer sample
  voice    an installed TTS voice, built from voice samples; not a sample itself
Reference: docs/SampleStore.md`
