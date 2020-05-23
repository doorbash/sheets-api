# remote-config
A simple API that reads key value config from Google Sheets.

## Install:

1. Create a new Google project. https://console.developers.google.com
2. Enable Google Sheets API for you project.
3. Add credentials to your project.
    You need to add an authorized redirect URI like `http://example.com:4040/callback`.
4. Download credentials file and save it as `credentials.json`.
5. `go get github.com/doorbash/remote-config`
6. Edit `main.go` and set `SPREADSHEET` const as your spreadsheet id.
7. `go build`
8. Put `credentials.json` next to `main.go`.
9. `./remote-config`
10. Visit `http://example.com:4040/login` and login with your Google account.

## Usage:

Put your data in two columns: (A=key, B=value)

<img src="https://github.com/doorbash/remote-config/blob/master/screenshot.png?raw=true" />

### Get all configs as JSON

```
http://example.com:4040/Sheet1/get

{
   "key1":"value1",
   "key10":"t",
   "key11":true,
   "key2":3.14,
   "key3":4,
   "key4":true,
   "key5":0,
   "key6":1,
   "key7":"",
   "key8":null,
   "key9":"\"true\""
}
```

### Get a specific key

```
http://example.com:4040/Sheet1/get?key=key4
    
true
```

## Example:

### Android:

```java
private class GetConfigAsyncTask extends AsyncTask<String, Integer, String> {
    protected String doInBackground(String... urls) {
        try {
            Log.d(TAG, "sending get request to server...");
            URL url = new URL(urls[0]);
            HttpURLConnection connection = (HttpURLConnection) url.openConnection();
            connection.setRequestMethod("GET");
            connection.setConnectTimeout(10000);
            connection.setReadTimeout(10000);
            connection.connect();
            int status = connection.getResponseCode();
            Log.d(TAG, "status code is " + status);
            if (status == HttpURLConnection.HTTP_OK) {
                InputStream is = connection.getInputStream();
                return new BufferedReader(new InputStreamReader(is)).readLine();
            } else {
                InputStream is = connection.getErrorStream();
                throw new Exception(new BufferedReader(new InputStreamReader(is)).readLine());
            }
        } catch (Exception e) {
            e.printStackTrace();
        }
        return null;
    }

    protected void onProgressUpdate(Integer... progress) {
    }

    protected void onPostExecute(String result) {
        if (result != null) {
            Log.d(TAG, "result is " + result);
            // Do something with the result here
        }
    }
}
```

```java
new GetConfigAsyncTask().execute("http://example.com:4040/Sheet1/get");
```

## License

MIT