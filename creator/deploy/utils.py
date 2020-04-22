def find_and_replace(data, find, replace):
    for key in data:
        if type(data) is list:
            for i in data:
                if find in data[i]:
                    data[i] = data[i].replace(find, replace)
        elif type(data[key]) is str:
            if find in data[key]:
                data[key] = data[key].replace(find, replace)
        elif type(data[key]) is dict:
            data[key] = find_and_replace(data[key], find, replace)
        elif type(data[key]) is list:
            for i in range(len(data[key])):
                if type(data[key][i]) is dict:
                    data[key][i] = find_and_replace(data[key][i], find, replace)
                elif type(data[key][i]) is str:
                    if find in data[key][i]:
                        data[key][i] = data[key][i].replace(find, replace)

    return data