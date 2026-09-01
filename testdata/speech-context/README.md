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
