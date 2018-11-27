import sys
import os
import subprocess

if __name__ == '__main__':
    # pre-check
    current_branch = subprocess.check_output(["git", "rev-parse", "--abbrev-ref", "HEAD"])
    current_branch = current_branch.replace("\n", "")
    if current_branch != "master":
        print("current branch is ", current_branch, " not master.")
        print("press `yes` to continue release, other words to abort")
        line = sys.stdin.readline()
        if line != "yes\n":
            print("abort release")
            sys.exit(1)

    if len(sys.argv) <= 1:
        result = subprocess.check_output(["make", "print-version"])
        print(result.replace("\n", ""))
        print("please pass in version as argument")
        sys.exit(1)
    # TODO get release version from passed-in param
    # TODO if not, get current version and plus 1
    version = sys.argv[1]
    print(version)
    if len(sys.argv) > 2:
        comment = sys.argv[2]
    else:
        comment = sys.argv[1]

    # use the version to do
    # `git tag -a "<version>" -m 'release <version>'`
    cmd = "git tag -a '{}' -m '{}'".format(version, comment)
    print(cmd)
    os.system(cmd)
    # `git push origin refs/tags/<version>`
    cmd = "git push origin refs/tags/{}".format(version)
    print(cmd)
    os.system(cmd)
