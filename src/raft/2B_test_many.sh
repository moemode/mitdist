mkdir 2btest
for i in {1..5}; do go test -run 2B | tee 2B_$i.out; done
for i in {1..5}; do go test -race -run 2B | tee 2B_$i_race.out; done