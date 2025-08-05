## Performance Regression Report

Even though the pointer-based optimization significantly improved the `getShard` function, the overall system performance **degraded**.


### Time Per Operation

| Metric  | Before      | After       | Change            |
| ------- | ----------- | ----------- | ----------------- |
| Time/op | 3.949 ns/op | 182.4 ns/op | **46× slower** |
| Change  | –           | –           | **+4,521%**       |


### Iterations Achieved

| Metric         | Before      | After     | Change          |
| -------------- | ----------- | --------- | --------------- |
| Iterations     | 291,123,867 | 6,225,157 | **–97.8%**      |
| Relative Speed | –           | –         | **\~47× fewer** |


### Diagnosis

The pointer optimization caused the `getShard` function to be mostly **optimized away**, shifting the **performance bottleneck** to `retrieveFromShard`.

As a result:

* `retrieveFromShard` now takes **200%–300% more time**.
* The system spends significantly more time in post-shard-retrieval logic.
* The apparent performance loss is due to **unintended work amplification** in the slower parts of the system.

