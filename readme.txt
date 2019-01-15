自己上传
1、切换到master分支
git checkout  master
git branch
2、删除不用的分支
git branch -D feature-en-docs-new
3、新建自己的本地分支
git checkout -b feature-en-docs-new -t origin/feature-en-docs
4、开始修改自己的东西
5、提交自己的修改
git add 修改的文件
git commit -m "change docs goods for supervisors"
git push origin feature-en-docs-new
6、选择合并到哪个分支 界面操作
7、合并完成后，可以点击删除自己创建的分支


下载指定分支：
git clone -b feature-en-docs-new https://github.com/moiot/moha.git