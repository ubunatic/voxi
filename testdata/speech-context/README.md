# Speech-context benchmark fixtures

`corpus.tsv` commits expectations and keyterms, but deliberately does not
commit voice recordings. Record each named WAV locally as 16 kHz mono PCM and
keep it in this directory. Use the same speaker, microphone, and phrasing for
both paired runs; the runner invokes the same file with and without the prompt.
WAV files under this directory are ignored by Git so private voice data is not
committed.

**Known gap (issue 176):** the benchmark now reads only the sample store layout
(`dictation/<id>.json` sidecars), so it does not read this directory's legacy
`corpus.tsv` until the fixtures are converted.

Run on your private samples (the default `-corpus` is the private sample store,
`~/.local/share/voxi/samples`, see `docs/SampleStore.md`):

```sh
go run ./scripts/speech_context_bench
```

Record samples with `voxi sample record <id>` or keep a dictation chunk with
`voxi sample add <id> --last`. The JSON reports transcript, WER, exact keyterm
recall, inference latency, and adjacent-word repetitions for each paired run.
Record hardware, `voxtype` version, model checksum/version, and aggregate
findings in issue 032.
