# Benchmarks: GPU vs CPU on Apple Silicon

Run with `kubectl apply -f examples/bench/torch.yaml` and `examples/bench/mlx.yaml`. Results are in `kubesilicon logs bench-torch` / `bench-mlx`.

**Method.** Each benchmark: 2 warmups, then 5 timed runs, median reported (min and max are in the log). GPU work is synchronized before the clock stops (`torch.mps.synchronize()`, `mx.eval`). Synthetic data, fp32, no downloads. One GPU job at a time, so the two scripts ran back to back.

**Machine.** Apple M5 Pro, 24 GB, macOS 27, torch 2.15.0, mlx 0.32.4. The kind cluster (3 nodes) and Docker Desktop were running, load average about 2, and the job runs at `nice 10`. Idle-machine numbers would likely be a bit better for both devices.

## PyTorch (MPS vs CPU)

| Benchmark | MPS | CPU | MPS speedup |
|---|---|---|---|
| matmul 1024x1024 | 2.0 ms (1085 GFLOPS) | 1.2 ms (1826 GFLOPS) | 0.6x (CPU wins) |
| matmul 2048x2048 | 3.4 ms (5081 GFLOPS) | 9.1 ms (1886 GFLOPS) | 2.7x |
| matmul 4096x4096 | 23.6 ms (5830 GFLOPS) | 79.3 ms (1733 GFLOPS) | 3.4x |
| ConvNet train step, batch 64, 3x32x32 | 8.7 ms (7344 img/s) | 67.7 ms (945 img/s) | 7.8x |
| Transformer encoder (4 layers, d=512, seq 128, batch 16), fwd+bwd | 41.5 ms (49.4k tok/s) | 182.1 ms (11.2k tok/s) | 4.4x |

## MLX (GPU vs CPU)

| Benchmark | GPU | CPU | GPU speedup |
|---|---|---|---|
| matmul 1024x1024 | 1.0 ms (2051 GFLOPS) | 1.1 ms (1916 GFLOPS) | 1.1x |
| matmul 2048x2048 | 1.3 ms (13364 GFLOPS) | 8.5 ms (2016 GFLOPS) | 6.5x |
| matmul 4096x4096 | 8.1 ms (17001 GFLOPS) | 68.4 ms (2009 GFLOPS) | 8.4x |
| MLP train step, batch 512, 784-1024-1024-10 | 1.8 ms (284k samples/s) | 6.7 ms (76k samples/s) | 3.7x |
| Transformer encoder (4 layers, d=512, seq 128, batch 16), fwd+bwd | 16.6 ms (123k tok/s) | 149.8 ms (13.7k tok/s) | 9.0x |

## Reading the numbers

- **Small work does not pay.** At 1024x1024 the CPU is as fast or faster: each GPU op has a fixed launch cost. The GPU wins once the work is big enough (2048 and up here).
- **MLX was faster than PyTorch MPS on the same matmul** (4096: 8.1 ms against 23.6 ms) and on the transformer step. Do not read this as "MLX is 3x faster": the transformer implementations differ in details (normalization, attention kernels), so those two rows are not a like-for-like model comparison. The matmul rows are the cleanest comparison.
- On the CPU, PyTorch reaches about 1.8 TFLOPS in fp32 and MLX about 2.0. Both are far from the 6 to 17 TFLOPS of the GPU.
- Earlier numbers in the README (one run, 8.2 ms against 69 ms; CNN 7.7 s against 40.5 s) are consistent with these.
- Not measured: fp16/bf16, memory use, power, long training runs, and thermal throttling over several minutes.
