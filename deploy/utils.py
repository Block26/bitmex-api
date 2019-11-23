def find_and_replace(data, find, replace):
    for key in data:
        if type(data[key]) is str:
            if find in data[key]:
                data[key] = data[key].replace(find, replace)
        elif type(data[key]) is dict:
            data[key] = find_and_replace(data[key], find, replace)
        elif type(data[key]) is list:
            for i in range(len(data[key])):
                if type(data[key][i]) is dict:
                    data[key][i] = find_and_replace(data[key][i], find, replace)

    return data