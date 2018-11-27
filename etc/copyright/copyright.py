#!/usr/bin/env python
# -*- coding: utf-8 -*-
from __future__ import unicode_literals

import argparse
import os
import tempfile


def update_copyright(fname, copyleft_txt, copyright_txt):
    t = tempfile.NamedTemporaryFile(mode='r+')
    with file(fname, 'r') as fio:
        fdata = fio.read()
        if copyleft_txt:
            if fdata.startswith(copyleft_txt):
                fdata = fdata[len(copyleft_txt):]
        if fdata.startswith(copyright_txt):
            t.close()
            return

        fdata = copyright_txt + fdata
        t.write(fdata)

    t.seek(0)

    print 'update', fname
    with file(fname, 'w') as fio:
        fio.write(t.read())

    t.close()


def traversal(dir_name, copyleft_txt, copyright_txt):
    fns = os.listdir(dir_name)
    for fn in fns:
        fullfn = os.path.join(dir_name, fn)
        if (os.path.isdir(fullfn)):
            traversal(fullfn, copyleft_txt, copyright_txt)
        elif (fullfn.endswith(".go")):
            update_copyright(fullfn, copyleft_txt, copyright_txt)


def main():
    arg_parser = argparse.ArgumentParser()
    arg_parser.add_argument('-o', '--copyleft', type=str, help='original copyright file')
    arg_parser.add_argument('copyright', type=str, help='copyright text file')
    arg_parser.add_argument('dir_name', type=str, help='target directory')
    args = arg_parser.parse_args()

    with file(args.copyright, 'r') as fin:
        copyright_txt = fin.read()

    if args.copyleft:
        with file(args.copyleft, 'r') as fin:
            copyleft_txt = fin.read()
    else:
        copyleft_txt = ''

    traversal(args.dir_name, copyleft_txt, copyright_txt)


if __name__ == '__main__':
    main()
