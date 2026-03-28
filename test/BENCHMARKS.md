# GenPool vs `sync.Pool`

## How to run

From the repo root (where `go.mod` is):

```bash
go test -bench . -benchmem ./test/
```

---

## What the levels mean


| Name        | innerIters | appendCount | Idea                          |
| ----------- | ---------: | ----------: | ----------------------------- |
| `pool_only` |          0 |           0 | Pool path only — minimal work |
| `low`       |        500 |          32 | Light CPU + small buffer      |
| `medium`    |     10_000 |         100 | Medium CPU + buffer           |
| `high`      |    100_000 |         256 | Heavy CPU + larger buffer     |
| `extreme`   |  1_000_000 |         256 | Max CPU; same append as high  |

---

## Sample results (one machine)

**Setup:** Windows, amd64, Intel Core Ultra 9 275HX, 24 logical CPUs.  
**Command:** `go test "-bench=." -benchmem ./test/ -count=5`  
**Table:** means over the five runs.

### Numbers

| Round     | Gen ns/op | Sync ns/op | Δ ns/op | Gen B/op | Sync B/op | Δ B/op | Gen allocs | Sync allocs |
| --------- | --------: | ---------: | :-----: | -------: | --------: | -----: | ---------: | ----------: |
| pool_only |      1.52 |      0.864 |  +77%   |        0 |         0 |      0 |          0 |           0 |
| low       |      62.6 |       62.7 |   ~0%   |        0 |         0 |      0 |          0 |           0 |
| medium    |      1223 |       1224 |   ~0%   |        2 |         2 |      0 |          0 |           0 |
| high      |     12179 |      12169 |   ~0%   |       23 |        24 |     +1 |          0 |           0 |
| extreme   |    120499 |     121071 |  -0.5%  |      225 |       222 |     -3 |          5 |         4.6 |


Lower **ns/op** is better.
