# Yantra Creator
### Setup
Run `pip install -r requirements.txt` to install the neccesary requirements 
### Create a Strategy
If you want to create a new strategy then you will need to run `python main.py` this will start a prompt of questions that will look like this

```What do you want to do?  create
What do you want to create?  strategy
What's the name of the strategy?  dx
Where should the files be placed?  /Users/russell/git/go/src/github.com/tantralabs
```

This will create a strategy called dx and place the files in `/Users/russell/git/go/src/github.com/tantralabs/dx` 
which should be the directory of the repo on your gopath. 

### Create Params
If you want to create a new params for a strategy then you will need to run `python main.py` this will start a prompt of questions that will look like this

```What do you want to do?  create
What do you want to create?  params
What's the name of the strategy?  dx
What's the name of the params?  dx-eth-vl
Where should the files be placed?  /Users/russell/git/go/src/github.com/tantralabs
```

This will create params called `dx-eth-vl` for the strategy `dx` and place the files in `/Users/russell/git/go/src/github.com/tantralabs/dx-eth-vl` 
which should be the directory of the repo on your gopath. 
