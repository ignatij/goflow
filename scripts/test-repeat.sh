#!/bin/bash

# Script to run make test 10 times and fail if any run fails
# Useful for detecting flaky tests or race conditions

set -e  # Exit on any error

echo "Running make test 10 times to check for flaky tests..."
echo "=================================================="

TOTAL_RUNS=10
FAILED_RUNS=0

for i in $(seq 1 $TOTAL_RUNS); do
    echo "Run $i/$TOTAL_RUNS"
    echo "----------------"
    
    if make test; then
        echo "✅ Run $i: PASSED"
    else
        echo "❌ Run $i: FAILED"
        FAILED_RUNS=$((FAILED_RUNS + 1))
    fi
    
    echo ""
done

echo "=================================================="
echo "Summary:"
echo "Total runs: $TOTAL_RUNS"
echo "Passed: $((TOTAL_RUNS - FAILED_RUNS))"
echo "Failed: $FAILED_RUNS"

if [ $FAILED_RUNS -gt 0 ]; then
    echo "❌ Some test runs failed! This indicates flaky tests or race conditions."
    exit 1
else
    echo "✅ All test runs passed! Tests appear to be stable."
    exit 0
fi 