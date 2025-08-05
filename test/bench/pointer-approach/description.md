## 🧪 Performance Regression Report

Despite the pointer-based optimization improving the `getShard` function in isolation, **overall system performance has regressed significantly**.


### ⏱ Time Per Operation

| Metric    | Baseline    | Optimized   | Change         |
| --------- | ----------- | ----------- | -------------- |
| Time/op   | 3.949 ns/op | 182.4 ns/op | **46× slower** |
| Delta (%) | –           | –           | **+4,521%**    |


### 🔁 Iteration Throughput

| Metric         | Baseline    | Optimized | Change              |
| -------------- | ----------- | --------- | ------------------- |
| Iterations     | 291,123,867 | 6,225,157 | **–97.8%**          |
| Relative Speed | –           | –         | **\~47× fewer ops** |


### 🔍 Diagnosis

While the pointer-based approach allowed the compiler to almost **fully optimize away `getShard`**, it also **exposed `retrieveFromShard` as the new bottleneck**. Key observations:

* `retrieveFromShard` now takes **2–3× longer** per call.
* More time is now spent in the **post-retrieval logic**, amplifying its cost.
* The regression stems from **unintended work shifting**, where performance-critical paths were previously masked by inefficiencies in `getShard`.


## 🔧 Code Changes

No major logic changes were introduced. The only notable modification:

* `getShard` now reads from a **static value** instead of a **global variable**.


### 📉 Additional Observation

Disabling `procPin` led to a dramatic slowdown.

* **Atomic operations** now consume **\~70% of total execution time**, compared to **24% in the baseline**.
* This indicates increased **CPU contention and synchronization overhead** in the optimized version.

