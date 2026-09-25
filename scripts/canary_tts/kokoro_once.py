"""One-shot CPU Kokoro adapter used only by benchmark.py."""
import os
import sys

import onnxruntime as ort
import numpy as np
import soundfile as sf
from kokoro_onnx import Kokoro

model = os.environ["VOXI_KOKORO_MODEL"]
voices = os.environ["VOXI_KOKORO_VOICES"]
options = ort.SessionOptions()
options.intra_op_num_threads = min(os.cpu_count() or 1, 4)
options.inter_op_num_threads = 1
session = ort.InferenceSession(model, sess_options=options, providers=["CPUExecutionProvider"])
engine = Kokoro.from_session(session, voices)
# kokoro-onnx 0.4.7 emits int32 for speed when it sees `input_ids`; the
# published kokoro-v1.0.onnx expects float32. Convert at the session boundary.
class SpeedDTypeCompat:
    def __init__(self, wrapped):
        self.wrapped = wrapped
        self.inputs = {item.name: item.type for item in wrapped.get_inputs()}

    def __getattr__(self, name):
        return getattr(self.wrapped, name)

    def run(self, output_names, input_feed, *args, **kwargs):
        if self.inputs.get("speed") == "tensor(float)":
            input_feed["speed"] = np.asarray(input_feed["speed"], dtype=np.float32)
        return self.wrapped.run(output_names, input_feed, *args, **kwargs)

engine.sess = SpeedDTypeCompat(engine.sess)
samples, rate = engine.create(
    "Hello. This is a short local speech synthesis benchmark.",
    voice=os.getenv("VOXI_KOKORO_VOICE", "af_sarah"),
    lang=os.getenv("VOXI_KOKORO_LANGUAGE", "en-us"),
)
sf.write(sys.argv[1], samples, rate)
