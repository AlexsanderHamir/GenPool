## Conclusion

Across all scenarios—single shard or many, high or low concurrency—the primary factor affecting performance was how quickly objects were returned. The less time spent using the object, the more likely `sync.Pool` was to outperform GenPool.
