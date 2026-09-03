# Speech-context benchmark corpus

`corpus.tsv` commits expectations and keyterms, but deliberately does not
commit voice recordings. Record each named WAV locally as 16 kHz mono PCM and
keep it in this directory. Use the same speaker, microphone, and phrasing for
both paired runs; the runner invokes the same file with and without the prompt.

Run:

```sh
go run ./scripts/speech_context_bench -corpus testdata/speech-context
```

The JSON reports transcript, WER, exact keyterm recall, inference latency, and
adjacent-word repetitions for each paired run. Record hardware, `voxtype`
version, model checksum/version, and aggregate findings in issue 032. WAV files
under this directory are ignored by Git so private voice data is not committed.

## Using recorded dev samples as an additional corpus

`voxi feedback sample record <name>` (see issue 042) records real microphone
utterances into `~/.config/voxi/samples/`, writing a manifest
(`~/.config/voxi/samples/corpus.tsv`) in exactly this directory's format. To
include those private recordings in a bench run, point `-corpus` at that
directory instead — no code changes needed:

```sh
go run ./scripts/speech_context_bench -corpus ~/.config/voxi/samples
```

Run it once per corpus (this directory's public fixtures, then your private
samples directory) and compare reports; the runner does not merge corpora in
one invocation.
