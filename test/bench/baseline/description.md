### Focus of Optimization: `getShard`

This report centers on optimizing the `getShard` function.

While `getShard` **can be optimized in isolation**, doing so has a **negative impact on overall system performance**. The optimization reduces its own cost but shifts the bottleneck elsewhere—ultimately resulting in a **net slowdown** of the system.
