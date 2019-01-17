自己上传
1、切换到master分支
git checkout  master
git branch
2、删除不用的分支
git branch -D feature-en-docs-new
3、新建自己的本地分支
git checkout -b feature-en-docs-new -t origin/feature-en-docs-new
4、开始修改自己的东西
5、提交自己的修改
git add 修改的文件
git commit -m "change docs goods for supervisors"
git push origin feature-en-docs-new
6、选择合并到哪个分支 界面操作
7、合并完成后，可以点击删除自己创建的分支


下载指定分支：
git clone -b feature-en-docs-new https://github.com/moiot/moha.git


如何正确监控MySQL主从复制延迟(请考虑5.6版本前后区别，即并行复制及GTID等因素)？

一、基于binlog和postion模式
通过观察io线程减去sql线程对比的方式对比： Master_Log_File == Relay_Master_Log_File && Read_Master_Log_Pos == Exec_Master_Log_Pos。
更严谨的做法是同时也要判断Master上的binlog file & position 最新状态。

二、基于GTID模式
1、通过接受事务数减去已经执行事物数对比：Retrieved_Gtid_Set == Executed_Gtid_Set。
./agent/service_manager.go:187:	currentPos.GTID = rSet["Executed_Gtid_Set"]
./agent/service_manager.go:241:	endTxnIDInt64, err := mysql.GetTxnIDFromGTIDStr(rSet["Executed_Gtid_Set"], rSet["Master_UUID"])
./agent/service_manager.go:246:	return rSet["Master_UUID"], rSet["Executed_Gtid_Set"], uint64(endTxnIDInt64), nil


基于并行复制
2、先通过P_S库replication_applier_status_by_coordinator和replication_applier_status_by_worker表来观察每个复制线程的状态，后配合postion复制或GTID复制方法来监控复制延迟。
