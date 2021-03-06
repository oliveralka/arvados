#!/usr/bin/env python
# Copyright (C) The Arvados Authors. All rights reserved.
#
# SPDX-License-Identifier: AGPL-3.0

import os
import sys
import setuptools.command.egg_info as egg_info_cmd

from setuptools import setup, find_packages

SETUP_DIR = os.path.dirname(__file__) or "."
README = os.path.join(SETUP_DIR, 'README.rst')

try:
    import gittaggers
    tagger = gittaggers.EggInfoFromGit
except ImportError:
    tagger = egg_info_cmd.egg_info

setup(name='arvados-node-manager',
      version='0.1',
      description='Arvados compute node manager',
      long_description=open(README).read(),
      author='Arvados',
      author_email='info@arvados.org',
      url="https://arvados.org",
      license='GNU Affero General Public License, version 3.0',
      packages=find_packages(),
      scripts=['bin/arvados-node-manager'],
      data_files=[
          ('share/doc/arvados-node-manager', ['agpl-3.0.txt', 'README.rst']),
      ],
      install_requires=[
          'apache-libcloud>=2.2',
          'arvados-python-client>=0.1.20170731145219',
          'future',
          'pykka',
          'python-daemon',
          'setuptools'
      ],
      dependency_links=[
          "https://github.com/curoverse/libcloud/archive/apache-libcloud-2.2.2.dev3.zip"
      ],
      test_suite='tests',
      tests_require=[
          'requests',
          'pbr<1.7.0',
          'mock>=1.0',
          'apache-libcloud==2.2.2.dev3',
      ],
      zip_safe=False,
      cmdclass={'egg_info': tagger},
      )
