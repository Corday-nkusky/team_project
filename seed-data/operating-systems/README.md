# 操作系统课程讲义

六份中文资料：概论、进程管理、虚拟内存基础、虚拟内存机制、处理器虚拟化和综合课程讲义。正文不含练习题、参考答案或采集备注，来源及授权见 [SOURCES.md](SOURCES.md)。其中两份由开放课程按主题译写，不是逐字全译。保留文件中的许可归属，MIT 衍生资料限非商业教学。

## 使用

先按根 README 启动服务和 worker，再从仓库根目录执行：

```sh
node seed-data/operating-systems/seed.mjs --check
node seed-data/operating-systems/seed.mjs
```

默认连接 http://127.0.0.1:8080，使用项目已有演示账号 teacher1。可通过 BASE、SEED_USER、SEED_PASS 指定服务和凭据。导入只接受班级 1 的教师，学生及其他班级账号在上传前被拒绝。不要把真实凭据写入文件或提交到 Git。

导入器复用 biology/seed.mjs：上传后轮询到索引成功；再次执行时同标题同正文的 ready 材料跳过，不重复上传。已有同标题但正文不同则失败，不会覆盖原文或自动修复旧版本；请联系项目维护者处理。导入中途失败可能已有部分成功，再次运行可继续。临时数据和运行清单退出时清理。

修改讲义后须同步更新 corpus.json 的 SHA-256 校验和（UTF-8 正文，换行统一为 LF 后计算）。运行 `node --test seed-data/operating-systems/seed.test.mjs` 验证数据和拒绝路径。
