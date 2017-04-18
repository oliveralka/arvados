#!/usr/bin/env python
# -*- coding: utf-8 -*-

import io
import os
import shutil
import tempfile

import arvados
import arvados.collection as collection
import arvados.commands.get as arv_get
import run_test_server

from arvados_testutil import redirected_streams

class ArvadosGetTestCase(run_test_server.TestCaseWithServers):
    MAIN_SERVER = {}
    KEEP_SERVER = {}

    def setUp(self):
        super(ArvadosGetTestCase, self).setUp()
        self.tempdir = tempfile.mkdtemp()
        self.col_loc, self.col_pdh, self.col_manifest = self.write_test_collection()

    def tearDown(self):
        super(ArvadosGetTestCase, self).tearDown()
        shutil.rmtree(self.tempdir)

    def write_test_collection(self,
                              strip_manifest=False,
                              contents = {
                                  'foo.txt' : 'foo',
                                  'bar.txt' : 'bar',
                                  'subdir/baz.txt' : 'baz',
                              }):
        c = collection.Collection()
        for path, data in contents.items():
            with c.open(path, 'w') as f:
                f.write(data)
        c.save_new()
        return (c.manifest_locator(),
                c.portable_data_hash(),
                c.manifest_text(strip=strip_manifest))
    
    def run_get(self, args):
        self.stdout = io.BytesIO()
        self.stderr = io.BytesIO()
        return arv_get.main(args, self.stdout, self.stderr)

    def test_version_argument(self):
        err = io.BytesIO()
        out = io.BytesIO()
        with redirected_streams(stdout=out, stderr=err):
            with self.assertRaises(SystemExit):
                self.run_get(['--version'])
        self.assertEqual(out.getvalue(), '')
        self.assertRegexpMatches(err.getvalue(), "[0-9]+\.[0-9]+\.[0-9]+")

    def test_get_single_file(self):
        # Get the file using the collection's locator
        r = self.run_get(["{}/subdir/baz.txt".format(self.col_loc), '-'])
        self.assertEqual(0, r)
        self.assertEqual('baz', self.stdout.getvalue())
        # Then, try by PDH
        r = self.run_get(["{}/subdir/baz.txt".format(self.col_pdh), '-'])
        self.assertEqual(0, r)
        self.assertEqual('baz', self.stdout.getvalue())        

    def test_get_multiple_files(self):
        # Download the entire collection to the temp directory
        r = self.run_get(["{}/".format(self.col_loc), self.tempdir])
        self.assertEqual(0, r)
        with open(os.path.join(self.tempdir, "foo.txt"), "r") as f:
            self.assertEqual("foo", f.read())
        with open(os.path.join(self.tempdir, "bar.txt"), "r") as f:
            self.assertEqual("bar", f.read())
        with open(os.path.join(self.tempdir, "subdir", "baz.txt"), "r") as f:
            self.assertEqual("baz", f.read())

    def test_get_collection_unstripped_manifest(self):
        # Get the collection manifest by UUID
        r = self.run_get([self.col_loc, self.tempdir])
        self.assertEqual(0, r)
        m_from_collection = collection.Collection(self.col_manifest).manifest_text(strip=True)
        with open(os.path.join(self.tempdir, self.col_loc), "r") as f:
            # Strip manifest before comparison to avoid races
            m_from_file = collection.Collection(f.read()).manifest_text(strip=True)
            self.assertEqual(m_from_collection, m_from_file)
        # Get the collection manifest by PDH
        r = self.run_get([self.col_pdh, self.tempdir])
        self.assertEqual(0, r)
        with open(os.path.join(self.tempdir, self.col_pdh), "r") as f:
            # Strip manifest before comparison to avoid races
            m_from_file = collection.Collection(f.read()).manifest_text(strip=True)
            self.assertEqual(m_from_collection, m_from_file)

    def test_get_collection_stripped_manifest(self):
        col_loc, col_pdh, col_manifest = self.write_test_collection(strip_manifest=True)
        # Get the collection manifest by UUID
        r = self.run_get(['--strip-manifest', col_loc, self.tempdir])
        self.assertEqual(0, r)
        with open(os.path.join(self.tempdir, col_loc), "r") as f:
            self.assertEqual(col_manifest, f.read())
        # Get the collection manifest by PDH
        r = self.run_get(['--strip-manifest', col_pdh, self.tempdir])
        self.assertEqual(0, r)
        with open(os.path.join(self.tempdir, col_pdh), "r") as f:
            self.assertEqual(col_manifest, f.read())

    def test_invalid_collection(self):
        # Asking for an invalid collection should generate an error.
        r = self.run_get(['this-uuid-seems-to-be-fake', self.tempdir])
        self.assertNotEqual(0, r)

    def test_invalid_file_request(self):
        # Asking for an inexistant file within a collection should generate an error.
        r = self.run_get(["{}/im-not-here.txt".format(self.col_loc), self.tempdir])
        self.assertNotEqual(0, r)

    def test_invalid_destination(self):
        # Asking to place the collection's files on a non existant directory
        # should generate an error.
        r = self.run_get([self.col_loc, "/fake/subdir/"])
        self.assertNotEqual(0, r)

    def test_preexistent_destination(self):
        # Asking to place a file with the same path as a local one should
        # generate an error and avoid overwrites.
        with open(os.path.join(self.tempdir, "foo.txt"), "w") as f:
            f.write("another foo")
        r = self.run_get(["{}/foo.txt".format(self.col_loc), self.tempdir])
        self.assertNotEqual(0, r)
        with open(os.path.join(self.tempdir, "foo.txt"), "r") as f:
            self.assertEqual("another foo", f.read())

