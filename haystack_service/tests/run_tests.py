#!/usr/bin/env python3
"""
Script to run the unit tests for the haystack service.
"""

import unittest
import sys
import os

# Add the parent directory to the path so we can import the haystack_service package
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), '../..')))

if __name__ == "__main__":
    # Discover and run all tests in the tests directory
    test_suite = unittest.defaultTestLoader.discover(
        start_dir=os.path.dirname(__file__),
        pattern='test_*.py'
    )
    
    # Run the tests
    result = unittest.TextTestRunner(verbosity=2).run(test_suite)
    
    # Exit with non-zero status if there were failures
    sys.exit(not result.wasSuccessful()) 