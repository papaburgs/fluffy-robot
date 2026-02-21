# New frontend idea

## Objective
- create go backed htmx frontend that utilizes the turso database collections instead of the local files.
- must be Mobile first and responsive so will work well on desktop as well

## Features
- all storage will be in turso cloud database, no local copies
- client preferences will be stored in browser storage like current version.
- frontend will be read only and must not change database
- all static files and css will be bundled in the binary, but flag will enable not doing that for development

## client preferences
- want to save client preferences in browser storage 
- store things related to what agents to include on the charts as current version does
- will store and make a config section for  'myagent' 
  - myagent is a preference setting so we can always include that agent, or highlight that agent name in the list.
- store some 'latest actions' like that last viewed chart so users do not need to change on reload
- want to have a client preferences page, that is entered with a gear icon, like the current one.
  - it will list all agents and let user select or deselect ones to view, can be kept the same for now, 
  - options to hide inactive (exactly 175000 credits) 
  - option to sort by credits or alphabetical
  - changes to that will update local storage immediatly, no save buttons required
  - use a dropdown to select 'myagent'
- when a user selects one of the chart options, save that locally so on new page load it will start with that one.

## resets
- the game resets every week, and the current reset is stored in the stats table of the database
- the go code can store that, and set a timer set to the next reset date (as found in the stats table) to refresh the reset date 5 mins after it is triggered
- all calls should have the ability to send 'reset' as a url parameter if we want a historical view


## efficientcy
- since this is run in a public cloud, we want to minimize memory usage. Make all queries as accurate and efficient as possible to only return rows as required and to 
zero out maps and arrays after used. More work can be put on the database.

## navigation
since this is mobile first, navigation will be in a menu bar on the left edge that can be hidden. for small screens it will be hidden by default and can be shown with a 'hamburger' button.
there can be an option for each of the chart options as well as one for leaderboard and preferences

## Pages
### charts
- 4 options for charts, last hour, last 4hr, last 24hr, and last 7 days
- credit values should be on both left and right axis of the chart.
- current go-echart implementation should be used again.
 
### leaderboard
the leaderboard page will show the latest leaderboard for the selected reset, a button can be there to switch between leaderboard of credits or charts

### stats
stats page shows details that are stored in the stats table

### jumpgates
the main jumpgates will show all the jumpgates being tracked in a table form
if a jumpgate is complete, change the cells to green with the completion time.
the table will show the jumpgate, the agents that exist in that system with completion status
clicking on the jumpgate will show a chart with fulfilled materials, they should be shown on separate axis by time.


