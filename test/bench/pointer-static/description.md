## Changes

There are no major code changes. The main difference is that `getShard` now reads from a static value instead of a global variable.

### Report

Unexpectedly, disabling `procPin` has led to a significant performance degradation.

Currently, **70% of the execution time** is consumed by atomic operations, compared to just **24% in the baseline**.
