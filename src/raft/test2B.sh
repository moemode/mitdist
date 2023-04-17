mkdir 2btest
for i in {1..5}; do go test -race -run 2B | tee 2btest/2B_race_$i.out; done
for i in {1..5}; do go test -run 2B | tee 2btest/2B_$i.out; done